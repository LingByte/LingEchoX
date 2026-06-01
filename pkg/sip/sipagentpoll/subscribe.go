package sipagentpoll

import (
	"sync"
	"sync/atomic"
)

// changeHub notifies SSE/long-poll subscribers when a seat's ringing state changes.
type changeHub struct {
	mu      sync.RWMutex
	nextID  atomic.Uint64
	subs    map[uint64]*subscription
	byACD   map[uint]*map[uint64]struct{} // acdTargetID -> sub IDs
}

type subscription struct {
	ch     chan uint
	acdIDs map[uint]struct{}
}

var hub = &changeHub{
	subs:  make(map[uint64]*subscription),
	byACD: make(map[uint]*map[uint64]struct{}),
}

// SubscribeACDChanges registers for updates on the given ACD pool row IDs.
// The channel receives the acdTargetID that changed; buffer may drop if slow.
func SubscribeACDChanges(acdTargetIDs []uint) (<-chan uint, func()) {
	ids := make(map[uint]struct{}, len(acdTargetIDs))
	for _, id := range acdTargetIDs {
		if id != 0 {
			ids[id] = struct{}{}
		}
	}
	if len(ids) == 0 {
		ch := make(chan uint)
		close(ch)
		return ch, func() {}
	}

	subID := hub.nextID.Add(1)
	ch := make(chan uint, 32)
	sub := &subscription{ch: ch, acdIDs: ids}

	hub.mu.Lock()
	hub.subs[subID] = sub
	for acdID := range ids {
		set := hub.byACD[acdID]
		if set == nil {
			set = &map[uint64]struct{}{}
			hub.byACD[acdID] = set
		}
		(*set)[subID] = struct{}{}
	}
	hub.mu.Unlock()

	cancel := func() {
		hub.mu.Lock()
		defer hub.mu.Unlock()
		sub, ok := hub.subs[subID]
		if !ok {
			return
		}
		delete(hub.subs, subID)
		for acdID := range sub.acdIDs {
			if set := hub.byACD[acdID]; set != nil {
				delete(*set, subID)
				if len(*set) == 0 {
					delete(hub.byACD, acdID)
				}
			}
		}
		close(sub.ch)
	}
	return ch, cancel
}

// NotifyACDTargetChanged signals subscribers that seat poll state may have changed.
func NotifyACDTargetChanged(acdTargetID uint) {
	if acdTargetID == 0 {
		return
	}
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	set := hub.byACD[acdTargetID]
	if set == nil {
		return
	}
	for subID := range *set {
		sub := hub.subs[subID]
		if sub == nil {
			continue
		}
		select {
		case sub.ch <- acdTargetID:
		default:
		}
	}
}
