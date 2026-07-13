// 全局事件总线：后端统一推送错误/警告/信息到前端 SSE。
package store

import (
	"encoding/json"
	"log"
	"sync"
	"time"
)

type Event struct {
	Level   string `json:"level"`
	Context string `json:"context,omitempty"`
	Message string `json:"message"`
	TS      int64  `json:"ts"`
}

type eventBus struct {
	mu          sync.Mutex
	subscribers map[chan []byte]struct{}
}

var globalBus = &eventBus{
	subscribers: map[chan []byte]struct{}{},
}

func Subscribe() chan []byte {
	ch := make(chan []byte, 64)
	globalBus.mu.Lock()
	globalBus.subscribers[ch] = struct{}{}
	globalBus.mu.Unlock()
	return ch
}

func Unsubscribe(ch chan []byte) {
	globalBus.mu.Lock()
	delete(globalBus.subscribers, ch)
	globalBus.mu.Unlock()
}

func Emit(level, context, message string) {
	evt := Event{
		Level:   level,
		Context: context,
		Message: message,
		TS:      time.Now().Unix(),
	}
	data, err := json.Marshal(evt)
	if err != nil {
		log.Printf("[events] marshal error: %v", err)
		return
	}
	globalBus.mu.Lock()
	defer globalBus.mu.Unlock()
	for ch := range globalBus.subscribers {
		select {
		case ch <- data:
		default:
		}
	}
}
