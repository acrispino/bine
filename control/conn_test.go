package control

import (
	"context"
	"fmt"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestConcurrentSendRequest(t *testing.T) {
	// Concurrent SendRequest calls share one connection, and Tor answers
	// commands in the order it reads them. Each call must return the reply
	// to its own command, and no call may wait forever for another.
	f := newFakeControl(t)

	const workers = 8
	const requests = 100
	start := make(chan struct{})
	errCh := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start
			for j := 0; j < requests; j++ {
				request := fmt.Sprintf("PING %d %d", worker, j)
				resp, err := f.client.SendRequest("%s", request)
				if err != nil {
					errCh <- err
					return
				}
				if resp.Reply != request {
					errCh <- fmt.Errorf("SendRequest(%q) received another request's reply: %q", request, resp.Reply)
					return
				}
			}
		}(i)
	}
	close(start)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent SendRequest deadlocked")
	}
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestSendRequestRelaysEventsBeforeReply(t *testing.T) {
	f := newFakeControl(t)
	events := make(chan Event, 1)
	if err := f.client.AddEventListener(events, EventCodeLogNotice); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := f.client.RemoveEventListener(events, EventCodeLogNotice); err != nil {
			t.Error(err)
		}
		close(events)
	}()

	const request = "PING before-reply"
	const eventData = "before reply"
	f.setEventsBeforeReply(request, "650 NOTICE "+eventData)
	resp, err := f.client.SendRequest("%s", request)
	if err != nil {
		t.Fatalf("SendRequest failed: %v", err)
	}
	if resp.Reply != request {
		t.Fatalf("SendRequest did not return its own reply: %#v", resp)
	}
	select {
	case event := <-events:
		notice, ok := event.(*LogEvent)
		if !ok || notice.Code() != EventCodeLogNotice || notice.Raw != eventData {
			t.Fatalf("unexpected relayed event: %#v", event)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SendRequest did not relay the event before its reply")
	}
}

func TestAddEventListenerReturnsWhenEventPrecedesReply(t *testing.T) {
	// AddEventListener must return even when Tor sends an event for an
	// already-subscribed code before it replies to the SETEVENTS command.
	f := newFakeControl(t)
	// Tor sends NOTICE events only after a SETEVENTS command that includes
	// NOTICE, so another listener subscribes first.
	subscribed := make(chan Event, 1)
	if err := f.client.AddEventListener(subscribed, EventCodeLogNotice); err != nil {
		t.Fatal(err)
	}

	// Nothing receives from listener until AddEventListener returns, as with
	// a caller that starts receiving only afterward. An event relayed to
	// listener before the SETEVENTS reply therefore has no receiver.
	//
	// The new listener also subscribes to WARN, which Tor is not yet
	// sending, so AddEventListener must send SETEVENTS. The fake server
	// sends the event before its reply to that command, which can list the
	// two codes in either order.
	listener := make(chan Event)
	for _, command := range []string{"SETEVENTS NOTICE WARN", "SETEVENTS WARN NOTICE"} {
		f.setEventsBeforeReply(command, "650 NOTICE before reply")
	}
	done := make(chan error, 1)
	go func() {
		done <- f.client.AddEventListener(listener, EventCodeLogNotice, EventCodeLogWarn)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("AddEventListener deadlocked relaying an event to its own listener")
	}

	// The new listener can receive the NOTICE or miss it while registration
	// completes. The relay may send the NOTICE to listener before subscribed,
	// so listener needs a receiver to avoid blocking the NOTICE for subscribed.
	received := make(chan Event, 1)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case event := <-listener:
				if notice, ok := event.(*LogEvent); ok && notice.Code() == EventCodeLogNotice {
					continue
				}
				received <- event
				return
			case <-stop:
				return
			}
		}
	}()

	// The fake server sends the event only before its reply to the
	// SETEVENTS command above. The event fails to arrive here in two cases:
	// AddEventListener did not send that command, so the test did not
	// exercise the deadlock, or Conn discarded the event instead of
	// relaying it.
	select {
	case event := <-subscribed:
		notice, ok := event.(*LogEvent)
		if !ok || notice.Raw != "before reply" {
			t.Fatalf("unexpected relayed event: %#v", event)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the event sent before the SETEVENTS reply was not relayed")
	}

	// WARN proves that the new listener was registered, even when it did not
	// receive the NOTICE sent before the SETEVENTS reply.
	handled := make(chan error, 1)
	go func() { handled <- f.client.HandleNextEvent() }()
	f.writeLines("650 WARN after reply")
	select {
	case event := <-received:
		warn, ok := event.(*LogEvent)
		if !ok || warn.Code() != EventCodeLogWarn || warn.Raw != "after reply" {
			t.Fatalf("unexpected event for the new listener: %#v", event)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the new listener did not receive WARN; registration or event delivery failed")
	}
	select {
	case err := <-handled:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("event handler did not finish after delivering WARN")
	}
}

func TestEventWaitWhileSending(t *testing.T) {
	// A synchronous request sent while EventWait waits for events must get
	// its reply, and EventWait must still return the event it waits for.
	//
	// EventWait's documentation says that it returns
	// ErrEventWaitSynchronousResponseOccurred when a non-event message
	// arrives, but no version of EventWait has returned that error. This
	// test checks what EventWait does: it keeps waiting while synchronous
	// requests complete.
	f := newFakeControl(t)
	conn := newObservedConn(f.clientConn)
	f.client = NewConn(textproto.NewConn(conn))
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	ready := make(chan struct{})
	eventCh := make(chan Event, 1)
	errCh := make(chan error, 1)
	go func() {
		event, err := f.client.EventWait(
			ctx,
			[]EventCode{EventCodeLogNotice},
			func(event Event) (bool, error) {
				if notice, ok := event.(*LogEvent); ok && notice.Raw == "ready" {
					close(ready)
					return false, nil
				}
				return true, nil
			},
		)
		if err != nil {
			errCh <- err
			return
		}
		eventCh <- event
	}()

	f.waitForCommand("SETEVENTS")
	// The only bytes the fake server has sent are its reply to the SETEVENTS
	// command that EventWait sent when it registered its listener. A Read
	// that started after those bytes and has not returned is the event
	// handler waiting for the next message.
	readDeadline := time.After(3 * time.Second)
	for {
		select {
		case read := <-conn.reads:
			if read.received < len("250 OK\r\n") {
				continue
			}
			select {
			case <-read.done:
				continue
			default:
			}
		case err := <-errCh:
			t.Fatal(err)
		case <-readDeadline:
			t.Fatal("event handler did not block reading after the SETEVENTS reply")
		}
		break
	}

	respCh := make(chan error, 1)
	go func() {
		resp, err := f.client.SendRequest("PING")
		if err == nil && (resp == nil || resp.Reply != "PING") {
			err = fmt.Errorf("SendRequest did not return the PING reply: %#v", resp)
		}
		respCh <- err
	}()
	select {
	case err := <-respCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SendRequest blocked while EventWait was active")
	}

	f.writeLines("650 NOTICE ready")
	select {
	case <-ready:
	case err := <-errCh:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal("timed out waiting for EventWait to relay the first event")
	}

	f.writeLines("650 NOTICE done")

	select {
	case event := <-eventCh:
		if notice, ok := event.(*LogEvent); !ok || notice.Raw != "done" {
			t.Fatalf("unexpected event: %#v", event)
		}
	case err := <-errCh:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal("timed out waiting for event")
	}
}

func TestSlowEventListenerReceivesEventsInOrder(t *testing.T) {
	// A registered listener receives every event in arrival order, even when
	// the listener starts receiving only after the events arrived.
	f := newFakeControl(t)
	listener := make(chan Event)
	if err := f.client.AddEventListener(listener, EventCodeLogNotice); err != nil {
		t.Fatal(err)
	}

	handled := make(chan struct{})
	var handleErr error
	const eventCount = 500
	go func() {
		defer close(handled)
		for range eventCount {
			if handleErr = f.client.HandleNextEvent(); handleErr != nil {
				return
			}
		}
	}()
	lines := make([]string, eventCount)
	for i := range lines {
		lines[i] = fmt.Sprintf("650 NOTICE event %d", i)
	}
	f.writeLines(lines...)

	// A relay that drops events can finish handling all events without a
	// receiver. The 100ms wait lets that relay finish before receiving starts.
	// The wait cannot fail a correct implementation because the test starts
	// receiving afterward without requiring the handler to have returned.
	select {
	case <-handled:
		if handleErr != nil {
			t.Fatal(handleErr)
		}
	case <-time.After(100 * time.Millisecond):
	}

	for i := range eventCount {
		want := fmt.Sprintf("event %d", i)
		select {
		case event := <-listener:
			notice, ok := event.(*LogEvent)
			if !ok || notice.Code() != EventCodeLogNotice || notice.Raw != want {
				t.Fatalf("unexpected event or order: want NOTICE %q, got %#v", want, event)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("NOTICE %q was dropped or never delivered", want)
		}
	}
	select {
	case <-handled:
		if handleErr != nil {
			t.Fatal(handleErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("event handler did not finish after delivering all events")
	}
}

func TestRemoveEventListenerUnblocksSender(t *testing.T) {
	// Removing a listener must not wait for anyone to read from its
	// channel. Here the event handler is sending an event to a listener
	// that nobody reads, and the test removes that listener. Once
	// RemoveEventListener returns, the caller may close the channel, as
	// AddEventListener's documentation allows.
	f := newFakeControl(t)

	first := make(chan Event)
	second := make(chan Event)
	for _, listener := range []chan Event{first, second} {
		if err := f.client.AddEventListener(listener, EventCodeUnrecognized); err != nil {
			t.Fatal(err)
		}
	}
	handled := make(chan error, 1)
	go func() { handled <- f.client.HandleNextEvent() }()

	f.writeLines("650 FOO bar")
	var remaining, removed chan Event
	var event Event
	select {
	case event = <-first:
		remaining, removed = first, second
	case event = <-second:
		remaining, removed = second, first
	case <-time.After(3 * time.Second):
		t.Fatal("event was not relayed to either listener")
	}
	if event.Code() != EventCode("FOO") {
		t.Fatalf("unexpected event code: %v", event.Code())
	}

	// Whatever traversal order the relay uses, it has sent FOO to one listener.
	// The relay is now sending, or about to send, FOO to removed, which has no
	// receiver.
	done := make(chan error, 1)
	go func() {
		done <- f.client.RemoveEventListener(removed, EventCodeUnrecognized)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RemoveEventListener deadlocked with a blocked listener")
	}

	close(removed)
	select {
	case err := <-handled:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("event handler did not finish after listener removal")
	}

	// Removing one listener must keep the other listener's registration for
	// EventCodeUnrecognized.
	go func() { handled <- f.client.HandleNextEvent() }()
	f.writeLines("650 BAR baz")
	select {
	case event := <-remaining:
		if event.Code() != EventCode("BAR") {
			t.Fatalf("unexpected event code after listener removal: %v", event.Code())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("event was not relayed after listener removal")
	}
	select {
	case err := <-handled:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("event handler did not finish the event after listener removal")
	}
}

func TestRemoveEventListenerUpdatesSubscriptions(t *testing.T) {
	// The last SETEVENTS command Tor read lists only codes that have registered
	// listeners. After the last listener is removed, the last SETEVENTS
	// command Tor read lists no codes.
	f := newFakeControl(t)
	notices := make(chan Event, 1)
	warnings := make(chan Event, 1)
	if err := f.client.AddEventListener(notices, EventCodeLogNotice); err != nil {
		t.Fatal(err)
	}
	if err := f.client.AddEventListener(warnings, EventCodeLogWarn); err != nil {
		t.Fatal(err)
	}

	if err := f.client.RemoveEventListener(notices, EventCodeLogNotice); err != nil {
		t.Fatal(err)
	}
	cmd := f.lastSetEvents()
	if codes := setEventsCodes(cmd); len(codes) != 1 || !codes[EventCodeLogWarn] {
		t.Fatalf("after removing the NOTICE listener, Tor's last SETEVENTS command was %q; want only WARN", cmd)
	}

	if err := f.client.RemoveEventListener(warnings, EventCodeLogWarn); err != nil {
		t.Fatal(err)
	}
	if cmd := f.lastSetEvents(); len(setEventsCodes(cmd)) != 0 {
		t.Fatalf("after removing the last listener, Tor's last SETEVENTS command was %q; want no codes", cmd)
	}
}

func TestSetEventsOmitsUnrecognizedPseudoCodeAndListsSubscribedCodes(t *testing.T) {
	// EventCodeUnrecognized is a pseudo-code that this library uses for
	// events it does not recognize. Tor does not know it, so SETEVENTS must
	// leave it out.
	//
	// SETEVENTS must still list codes that callers subscribe to but this
	// library does not recognize. Those subscriptions are how an
	// EventCodeUnrecognized listener receives events.
	f := newFakeControl(t)

	ch := make(chan Event, 1)
	defer close(ch)
	const futureEvent = EventCode("FUTURE_EVENT")
	events := []EventCode{EventCodeUnrecognized, EventCodeCircuit, futureEvent}
	if err := f.client.AddEventListener(ch, events...); err != nil {
		t.Fatal(err)
	}
	cmd := f.waitForCommand("SETEVENTS")
	sentCircuit := false
	sentFuture := false
	for _, field := range strings.Fields(cmd)[1:] {
		switch EventCode(field) {
		case EventCodeUnrecognized:
			t.Fatalf("unrecognized pseudo-event was sent to Tor: %q", cmd)
		case EventCodeCircuit:
			sentCircuit = true
		case futureEvent:
			sentFuture = true
		}
	}
	if !sentCircuit {
		t.Fatalf("recognized event %s was not sent to Tor: %q", EventCodeCircuit, cmd)
	}
	if !sentFuture {
		t.Fatalf("SETEVENTS left out event code %s, which this library does not recognize: %q", futureEvent, cmd)
	}
	if err := f.client.RemoveEventListener(ch, events...); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentListenerChangesKeepSubscriptions(t *testing.T) {
	// Tor keeps the event codes from the last SETEVENTS command it reads.
	// Concurrent listener changes can therefore leave Tor with an outdated
	// subscription: one call builds its SETEVENTS command from the listener
	// map, another call changes the map and writes its own command, and then
	// the first call writes its outdated command.
	//
	// The first case uses SendRequest's debug log line to hold a command
	// between building and writing it. The second case repeats concurrent
	// changes to exercise the race through parallel scheduling.
	t.Run("command held between building and writing", func(t *testing.T) {
		f := newFakeControl(t)
		// Pausing the first SETEVENTS log line holds the first
		// AddEventListener call's command after the call has built it and
		// before Tor reads it, which forces the race on every run.
		//
		// With parallel goroutines, the repeated case below makes the race likely.
		// It misses the race when every change to the listener map finishes before
		// any call builds its command.
		//
		// If the log line moves after the write, this case passes without
		// exercising the race. If SendRequest stops logging the command, this
		// case is skipped.
		//
		// In both situations, detecting the race depends on the repeated case.
		// That case does not reliably catch the race unless goroutines run in
		// parallel.
		w := newPausingWriter("SETEVENTS")
		t.Cleanup(w.Release)
		f.client.DebugWriter = w

		a := make(chan Event, 1)
		b := make(chan Event, 1)
		aErr := make(chan error, 1)
		go func() { aErr <- f.client.AddEventListener(a, EventCodeLogNotice) }()
		select {
		case <-w.paused:
		case err := <-aErr:
			if err != nil {
				t.Fatal(err)
			}
			t.Skip("AddEventListener returned without logging its SETEVENTS command, so the command cannot be held")
		case <-time.After(3 * time.Second):
			t.Fatal("AddEventListener neither logged its SETEVENTS command nor returned")
		}

		// Listener b registers while a's command is held. An implementation
		// may make b's command wait for a's, so b gets a bounded time to
		// finish before a is released.
		bErr := make(chan error, 1)
		go func() { bErr <- f.client.AddEventListener(b, EventCodeLogWarn) }()
		bDone := false
		select {
		case err := <-bErr:
			if err != nil {
				t.Fatal(err)
			}
			bDone = true
		case <-time.After(time.Second):
		}
		w.Release()
		pending := []chan error{aErr}
		if !bDone {
			pending = append(pending, bErr)
		}
		for _, errCh := range pending {
			select {
			case err := <-errCh:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("AddEventListener did not return")
			}
		}

		cmd := f.lastSetEvents()
		if codes := setEventsCodes(cmd); !codes[EventCodeLogNotice] || !codes[EventCodeLogWarn] {
			t.Fatalf("after both listeners were added, Tor's last SETEVENTS command was %q", cmd)
		}
	})

	// At any GOMAXPROCS setting, each round checks Tor's final subscription
	// after concurrent additions and removals and requires every change to
	// return. With -race, the subtest also checks for races on the listener map.
	//
	// The first subtest misses an implementation that builds SETEVENTS before
	// taking the lock that serializes writes, because its pause then holds
	// that lock. Parallel goroutines let this subtest exercise that race.
	// With GOMAXPROCS=1, this subtest does not reliably catch that
	// implementation because goroutines do not run in parallel.
	t.Run("repeated concurrent changes", func(t *testing.T) {
		f := newFakeControl(t)

		codes := []EventCode{EventCodeLogDebug, EventCodeLogInfo, EventCodeLogNotice, EventCodeLogWarn}
		listeners := make([]chan Event, len(codes))
		for i := range listeners {
			listeners[i] = make(chan Event, 1)
		}
		// change adds or removes all listeners concurrently, each with its
		// own event code.
		change := func(add bool) {
			start := make(chan struct{})
			errCh := make(chan error, len(codes))
			for i, code := range codes {
				go func(listener chan Event, code EventCode) {
					<-start
					if add {
						errCh <- f.client.AddEventListener(listener, code)
					} else {
						errCh <- f.client.RemoveEventListener(listener, code)
					}
				}(listeners[i], code)
			}
			close(start)
			for range codes {
				select {
				case err := <-errCh:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("concurrent listener change timed out; a concurrent SendRequest deadlock also produces this timeout (see TestConcurrentSendRequest)")
				}
			}
		}

		// The fake server stops reading commands once cmdCh is full, and this
		// case sends more commands than cmdCh holds, so each phase discards
		// the commands recorded so far.
		drain := func() {
			for {
				select {
				case <-f.cmdCh:
				default:
					return
				}
			}
		}

		// Whether the calls race depends on scheduling, so the case repeats
		// them.
		for round := 1; round <= 1000; round++ {
			change(true)
			drain()
			if cmd := f.lastSetEvents(); len(setEventsCodes(cmd)) != len(codes) {
				t.Fatalf("round %d: after all listeners were added, Tor's last SETEVENTS command was %q", round, cmd)
			}
			change(false)
			drain()
			if cmd := f.lastSetEvents(); len(setEventsCodes(cmd)) != 0 {
				t.Fatalf("round %d: after all listeners were removed, Tor's last SETEVENTS command was %q", round, cmd)
			}
		}
	})
}

type fakeControl struct {
	t                 *testing.T
	ln                net.Listener
	client            *Conn
	clientConn        net.Conn
	serverConn        net.Conn
	serverTP          *textproto.Conn
	mu                sync.Mutex
	cmdCh             chan string
	eventsBeforeReply map[string][]string
	setEvents         string
}

func newFakeControl(t *testing.T) *fakeControl {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeControl{t: t, ln: ln, cmdCh: make(chan string, 1000)}
	accepted := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			close(accepted)
			return
		}
		f.serverConn = conn
		f.serverTP = textproto.NewConn(conn)
		close(accepted)
		f.serve()
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	f.clientConn = conn
	f.client = NewConn(textproto.NewConn(conn))
	<-accepted
	if f.serverTP == nil {
		t.Fatal("fake control server failed to accept")
	}
	t.Cleanup(func() {
		// Close sends QUIT through SendRequest, which cannot finish after a
		// request deadlock. Closing the sockets lets cleanup finish instead.
		_ = f.clientConn.Close()
		_ = f.ln.Close()
		if f.serverConn != nil {
			_ = f.serverConn.Close()
		}
	})
	return f
}

// lastSetEvents returns the last SETEVENTS command the server read, or ""
// if it has read none.
func (f *fakeControl) lastSetEvents() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.setEvents
}

func (f *fakeControl) serve() {
	for {
		line, err := f.serverTP.ReadLine()
		if err != nil {
			return
		}
		f.mu.Lock()
		lines := append([]string(nil), f.eventsBeforeReply[line]...)
		if strings.HasPrefix(line, "SETEVENTS") {
			f.setEvents = line
		}
		f.mu.Unlock()
		reply := "250 OK"
		if strings.HasPrefix(line, "PING") {
			reply = "250 " + line
		}
		f.writeLines(append(lines, reply)...)
		f.cmdCh <- line
	}
}

func (f *fakeControl) setEventsBeforeReply(command string, lines ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.eventsBeforeReply == nil {
		f.eventsBeforeReply = make(map[string][]string)
	}
	f.eventsBeforeReply[command] = append([]string(nil), lines...)
}

func (f *fakeControl) waitForCommand(prefix string) string {
	f.t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case cmd := <-f.cmdCh:
			if strings.HasPrefix(cmd, prefix) {
				return cmd
			}
		case <-deadline:
			f.t.Fatalf("timed out waiting for command %q", prefix)
			return ""
		}
	}
}

func (f *fakeControl) writeLines(lines ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, line := range lines {
		if err := f.serverTP.PrintfLine("%s", line); err != nil {
			return
		}
	}
}

// observedConn reports each Read on reads before the underlying Read can
// block. Notifications are dropped when reads is full. Read calls must be
// serialized.
type observedConn struct {
	net.Conn
	reads    chan readAttempt
	received int
}

func newObservedConn(conn net.Conn) *observedConn {
	return &observedConn{Conn: conn, reads: make(chan readAttempt, 16)}
}

func (c *observedConn) Read(p []byte) (int, error) {
	read := readAttempt{received: c.received, done: make(chan struct{})}
	select {
	case c.reads <- read:
	default:
	}
	n, err := c.Conn.Read(p)
	c.received += n
	close(read.done)
	return n, err
}

// pausingWriter blocks the first Write whose data contains substr until
// Release is called. paused closes when that Write begins. The writer
// discards all data, including the paused write's.
type pausingWriter struct {
	substr  string
	paused  chan struct{}
	release chan struct{}
	once    sync.Once
	mu      sync.Mutex
	armed   bool
}

func newPausingWriter(substr string) *pausingWriter {
	return &pausingWriter{
		substr:  substr,
		paused:  make(chan struct{}),
		release: make(chan struct{}),
		armed:   true,
	}
}

// Release unblocks the paused Write. It may be called more than once.
func (w *pausingWriter) Release() {
	w.once.Do(func() { close(w.release) })
}

func (w *pausingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	pause := w.armed && strings.Contains(string(p), w.substr)
	if pause {
		w.armed = false
	}
	w.mu.Unlock()
	if pause {
		close(w.paused)
		<-w.release
	}
	return len(p), nil
}

// readAttempt describes a Read reported by observedConn.
type readAttempt struct {
	// received is the number of bytes the connection delivered before this
	// Read began.
	received int
	// done closes when this Read returns.
	done chan struct{}
}

// setEventsCodes returns the set of event codes that a SETEVENTS command
// lists. The set is empty when command is "".
func setEventsCodes(command string) map[EventCode]bool {
	codes := map[EventCode]bool{}
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return codes
	}
	for _, field := range fields[1:] {
		codes[EventCode(field)] = true
	}
	return codes
}
