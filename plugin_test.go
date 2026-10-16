package netmon

import (
	"context"
	"io/fs"
	"net/http"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
	_ "time/tzdata"

	"github.com/avanha/pmaas-plugin-netmon/config"
	netmonevents "github.com/avanha/pmaas-plugin-netmon/events"
	spi "github.com/avanha/pmaas-spi"
	"github.com/avanha/pmaas-spi/alert"
)

// These run the real plugin and its real monitoring tasks, polling a host that is only a switch, on the virtual clock of a
// testing/synctest bubble: hours of polls take a moment, and the time of each is exact.

type switchableProber struct {
	up atomic.Bool

	mu    sync.Mutex
	polls []time.Time
}

func newProber(up bool) *switchableProber {
	p := &switchableProber{}
	p.up.Store(up)

	return p
}

func (p *switchableProber) Reachable(context.Context) bool {
	p.mu.Lock()
	p.polls = append(p.polls, time.Now())
	p.mu.Unlock()

	return p.up.Load()
}

func (p *switchableProber) pollTimes() []time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()

	return append([]time.Time(nil), p.polls...)
}

// fakeContainer has the plugin goroutine as a goroutine of its own, and notes what is broadcast and when.
type fakeContainer struct {
	spi.IPMAASContainer

	work chan func()

	mu         sync.Mutex
	broadcasts []broadcast
	registered int
}

type broadcast struct {
	at    time.Time
	id    string
	event any
}

func newFakeContainer(t *testing.T) *fakeContainer {
	t.Helper()

	c := &fakeContainer{work: make(chan func(), 64)}

	go func() {
		for f := range c.work {
			f()
		}
	}()

	t.Cleanup(func() { close(c.work) })

	return c
}

func (c *fakeContainer) EnqueueOnPluginGoRoutine(f func()) error {
	c.work <- f

	return nil
}

func (c *fakeContainer) ProvideContentFS(fs.FS, string)                                 {}
func (c *fakeContainer) EnableStaticContent(string)                                     {}
func (c *fakeContainer) AddRoute(string, http.HandlerFunc)                              {}
func (c *fakeContainer) RegisterEntityRenderer(reflect.Type, spi.EntityRendererFactory) {}

func (c *fakeContainer) RegisterEntity(uniqueData string, _ reflect.Type, _ string, _ spi.EntityStubFactoryFunc) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.registered++

	return "netmon_" + uniqueData, nil
}

func (c *fakeContainer) DeregisterEntity(string) error { return nil }

func (c *fakeContainer) BroadcastEvent(id string, event any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.broadcasts = append(c.broadcasts, broadcast{at: time.Now(), id: id, event: event})

	return nil
}

func (c *fakeContainer) alerts() (raised []broadcast, cleared []broadcast) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, b := range c.broadcasts {
		switch b.event.(type) {
		case alert.RaisedEvent:
			raised = append(raised, b)
		case alert.ClearedEvent:
			cleared = append(cleared, b)
		}
	}

	return raised, cleared
}

func (c *fakeContainer) reachabilityChanges() []netmonevents.HostReachabilityChangeEvent {
	c.mu.Lock()
	defer c.mu.Unlock()

	var result []netmonevents.HostReachabilityChangeEvent

	for _, b := range c.broadcasts {
		if change, ok := b.event.(netmonevents.HostReachabilityChangeEvent); ok {
			result = append(result, change)
		}
	}

	return result
}

// started is a plugin watching the hosts in the configuration, running on the bubble's clock.
func started(t *testing.T, conf config.PluginConfig) (*plugin, *fakeContainer) {
	t.Helper()

	container := newFakeContainer(t)
	p := NewPlugin(conf).(*plugin)

	p.Init(container)
	p.Start()
	synctest.Wait()

	return p, container
}

func stop(t *testing.T, p *plugin) {
	t.Helper()

	done := p.Stop()

	callback, open := <-done
	if open {
		callback()
	}

	synctest.Wait()
}

func aLittleLater(d time.Duration) {
	time.Sleep(d)
	synctest.Wait()
}

func TestUnreachableAlert_RaisedAfterTwoMinutesDownAndClearedWhenTheHostIsBack(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		prober := newProber(true)

		conf := config.PluginConfig{}
		conf.AddHost("router", "10.0.0.1").UseProber(prober).AlertWhenUnreachable(2*time.Minute, nil)

		p, container := started(t, conf)

		// Ten minutes of the host being up: no alert.
		aLittleLater(10 * time.Minute)

		if raised, _ := container.alerts(); len(raised) != 0 {
			t.Fatalf("an alert for a host that is up: %+v", raised)
		}

		// The host goes down. The first poll that finds it so is when it became unreachable.
		before := len(prober.pollTimes())
		prober.up.Store(false)

		aLittleLater(10 * time.Minute)

		polls := prober.pollTimes()[before:]
		down := polls[0]
		raised, _ := container.alerts()

		if len(raised) != 1 {
			t.Fatalf("%d alerts raised: %+v", len(raised), raised)
		}

		event := raised[0].event.(alert.RaisedEvent)
		if event.Source != "netmon" || event.Key != "host-unreachable:router" || event.Severity != alert.SeverityCritical ||
			event.Title != "Host unreachable: router" {
			t.Errorf("raised %+v", event)
		}

		// It came at the first poll two minutes after the first that found it down, and not a poll sooner.
		if want := down.Add(2 * time.Minute); !raised[0].at.Equal(want) {
			t.Errorf("raised at %v, want %v (down since %v)", raised[0].at, want, down)
		}

		// It's back.
		prober.up.Store(true)
		backBefore := len(prober.pollTimes())

		aLittleLater(5 * time.Minute)

		_, cleared := container.alerts()
		back := prober.pollTimes()[backBefore]

		if len(cleared) != 1 || cleared[0].event.(alert.ClearedEvent).Key != "host-unreachable:router" || !cleared[0].at.Equal(back) {
			t.Errorf("cleared %+v at the poll of %v", cleared, back)
		}

		if raised, _ := container.alerts(); len(raised) != 1 {
			t.Errorf("raised again: %+v", raised)
		}

		stop(t, p)
	})
}

func TestUnreachableAlert_AnOutageShorterThanTheTimeRaisesNothing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		prober := newProber(true)

		conf := config.PluginConfig{}
		conf.AddHost("router", "10.0.0.1").UseProber(prober).AlertWhenUnreachable(5*time.Minute, nil)

		p, container := started(t, conf)

		aLittleLater(5 * time.Minute)
		prober.up.Store(false)
		aLittleLater(3 * time.Minute) // three polls or so, less than five minutes
		prober.up.Store(true)
		aLittleLater(10 * time.Minute)

		raised, cleared := container.alerts()
		if len(raised) != 0 || len(cleared) != 0 {
			t.Errorf("raised %+v, cleared %+v", raised, cleared)
		}

		// But it did go unreachable and come back: the events say so, alert or not.
		changes := container.reachabilityChanges()
		if len(changes) < 3 {
			t.Errorf("%d reachability changes", len(changes))
		}

		stop(t, p)
	})
}

func TestUnreachableAlert_EscalatesWhenTheNightComesWhileTheHostIsStillDown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		denver, err := time.LoadLocation("America/Denver")
		if err != nil {
			t.Fatal(err)
		}

		previous := time.Local
		time.Local = denver

		t.Cleanup(func() { time.Local = previous })

		time.Sleep(time.Until(time.Date(2000, time.June, 21, 21, 0, 0, 0, denver)))

		prober := newProber(true)

		conf := config.PluginConfig{}
		conf.AddHost("router", "10.0.0.1").UseProber(prober).
			AlertWhenUnreachable(2*time.Minute, alert.ByTimeOfDay(alert.SeverityWarning, 7, 0, 23, 0, alert.SeverityCritical))

		p, container := started(t, conf)

		aLittleLater(30 * time.Minute) // 21:30
		prober.up.Store(false)
		aLittleLater(5 * time.Hour) // until 02:30 the next morning

		raised, _ := container.alerts()
		if len(raised) != 2 {
			t.Fatalf("%d alerts raised: %+v", len(raised), raised)
		}

		first, second := raised[0].event.(alert.RaisedEvent), raised[1].event.(alert.RaisedEvent)
		if first.Severity != alert.SeverityWarning || second.Severity != alert.SeverityCritical || first.Key != second.Key {
			t.Errorf("severities %v then %v, keys %q and %q", first.Severity, second.Severity, first.Key, second.Key)
		}

		// The escalation came at the first poll at or after 11pm.
		if at := raised[1].at; at.Hour() != 23 || at.Minute() != 0 {
			t.Errorf("escalated at %v", at.Format("15:04:05"))
		}

		// And it eases at 7am if the host is still down.
		aLittleLater(5 * time.Hour) // 07:30

		raised, _ = container.alerts()
		if len(raised) != 3 || raised[2].event.(alert.RaisedEvent).Severity != alert.SeverityWarning {
			t.Fatalf("after 7am: %+v", raised)
		}

		if at := raised[2].at; at.Hour() != 7 || at.Minute() != 0 {
			t.Errorf("eased at %v", at.Format("15:04:05"))
		}

		stop(t, p)
	})
}

func TestUnreachableAlert_OnlyForTheHostsThatAskedForOne(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		alertingProber, quietProber := newProber(false), newProber(false)

		conf := config.PluginConfig{}
		conf.AddHost("watched", "10.0.0.1").UseProber(alertingProber).AlertWhenUnreachable(time.Minute, nil)
		conf.AddHost("unwatched", "10.0.0.2").UseProber(quietProber)

		p, container := started(t, conf)

		aLittleLater(10 * time.Minute)

		raised, _ := container.alerts()
		if len(raised) != 1 || raised[0].event.(alert.RaisedEvent).Key != "host-unreachable:watched" {
			t.Errorf("raised %+v", raised)
		}

		stop(t, p)
	})
}

func TestAddHost_ThePointerItReturnsIsToTheHostThePluginWillUseHoweverManyAreAddedAfter(t *testing.T) {
	conf := config.PluginConfig{}

	first := conf.AddHost("first", "10.0.0.1")

	for i := 0; i < 20; i++ {
		conf.AddHost("other", "10.0.0.2")
	}

	// Adjusted after the others were added: it must still be the first host that is changed.
	first.PingEnabled = false
	first.AlertWhenUnreachable(time.Minute, nil)

	if conf.Hosts[0] != first || conf.Hosts[0].PingEnabled {
		t.Error("the pointer was to a copy")
	}

	if _, _, wanted := conf.Hosts[0].UnreachableAlert(); !wanted {
		t.Error("the alert setting was lost")
	}

	if _, _, wanted := conf.Hosts[1].UnreachableAlert(); wanted {
		t.Error("another host got the setting")
	}
}

func TestAlertWhenUnreachable_Settings(t *testing.T) {
	conf := config.PluginConfig{}
	host := conf.AddHost("router", "10.0.0.1")

	if _, _, wanted := host.UnreachableAlert(); wanted {
		t.Error("an alert by default")
	}

	host.AlertWhenUnreachable(90*time.Second, nil)

	after, severity, wanted := host.UnreachableAlert()
	if !wanted || after != 90*time.Second || severity(time.Now()) != alert.SeverityCritical {
		t.Errorf("after %v, severity %v, wanted %v", after, severity(time.Now()), wanted)
	}

	for _, bad := range []time.Duration{0, -time.Minute} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%v didn't panic", bad)
				}
			}()

			host.AlertWhenUnreachable(bad, nil)
		}()
	}
}

func TestUnreachableAlert_AHostThatIsOnlyPingedWorksAsWellAsOneThatIsAskedOverSnmp(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		prober := newProber(false)

		conf := config.PluginConfig{}
		host := conf.AddHost("router", "10.0.0.1").UseProber(prober).AlertWhenUnreachable(time.Minute, nil)
		host.SnmpEnabled = false

		p, container := started(t, conf)

		aLittleLater(10 * time.Minute)

		if raised, _ := container.alerts(); len(raised) != 1 {
			t.Errorf("raised %+v", raised)
		}

		prober.up.Store(true)
		aLittleLater(5 * time.Minute)

		if _, cleared := container.alerts(); len(cleared) != 1 {
			t.Errorf("cleared %+v", cleared)
		}

		stop(t, p)
	})
}
