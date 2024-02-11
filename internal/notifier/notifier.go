package notifier

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type Notifier struct {
	mu      *sync.RWMutex
	wsConns map[userDevice]*chan any
	pushCh  chan any
}

func NewNotifier(pushWorkerPoolSize int) (*Notifier, error) {
	if pushWorkerPoolSize < 0 {
		return nil, errors.New("worker pool cannot have negative size")
	}

	pushCh := make(chan any, 100)
	for i := 0; i < pushWorkerPoolSize; i++ {
		go func() {
			for e := range pushCh {
				fmt.Printf("sending push: %+v\n", e)
			}
		}()
	}

	return &Notifier{
		mu:      &sync.RWMutex{},
		wsConns: make(map[userDevice]*chan any),
		pushCh:  pushCh,
	}, nil
}

type userDevice struct {
	userID      string
	deviceToken string
}

func (n *Notifier) UnregisterUser(ctx context.Context, userID string, deviceToken string) {
	key := userDevice{
		userID:      userID,
		deviceToken: deviceToken,
	}

	n.mu.Lock()
	if chPtr, ok := n.wsConns[key]; ok {
		*chPtr = nil
	}
	delete(n.wsConns, key)
	n.mu.Unlock()
}

func (n *Notifier) RegisterUser(ctx context.Context, userID string, deviceToken string) chan any {
	key := userDevice{
		userID:      userID,
		deviceToken: deviceToken,
	}

	n.mu.RLock()
	if ch, ok := n.wsConns[key]; ok && *ch != nil {
		n.mu.RUnlock()
		return *ch
	}
	n.mu.RUnlock()

	n.mu.Lock()
	ch := make(chan any, 5)
	n.wsConns[key] = &ch
	n.mu.Unlock()

	return ch
}

func (n *Notifier) sendAsPush(ctx context.Context, key userDevice, event any) {
	n.pushCh <- struct {
		UserID      string
		DeviceToken string
		Event       any
	}{
		UserID:      key.userID,
		DeviceToken: key.deviceToken,
		Event:       event,
	}
}

func (n *Notifier) BroadcastEvent(ctx context.Context, userID string, deviceTokens []string, event any) error {
	for _, token := range deviceTokens {
		key := userDevice{
			userID:      userID,
			deviceToken: token,
		}
		n.mu.RLock()
		ch, ok := n.wsConns[key]
		n.mu.RUnlock()
		if !ok || *ch == nil {
			n.sendAsPush(ctx, key, event)
			continue
		}
		select {
		case *ch <- event:
		default:
			n.sendAsPush(ctx, key, event)
			n.UnregisterUser(ctx, userID, token)
		}
	}
	return nil
}
