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
	history     [][]byte
}

// historyLimit 回放给新订阅者的历史事件条数上限
const historyLimit = 50

var globalBus = &eventBus{
	subscribers: map[chan []byte]struct{}{},
}

func Subscribe() chan []byte {
	ch := make(chan []byte, 64)
	globalBus.mu.Lock()
	defer globalBus.mu.Unlock()
	// 回放订阅前的事件（如启动期的错误/警告）
	for _, data := range globalBus.history {
		select {
		case ch <- data:
		default:
		}
	}
	globalBus.subscribers[ch] = struct{}{}
	return ch
}

func Unsubscribe(ch chan []byte) {
	globalBus.mu.Lock()
	delete(globalBus.subscribers, ch)
	close(ch)
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
	globalBus.history = append(globalBus.history, data)
	if len(globalBus.history) > historyLimit {
		globalBus.history = globalBus.history[len(globalBus.history)-historyLimit:]
	}
	for ch := range globalBus.subscribers {
		select {
		case ch <- data:
		default:
		}
	}
}
