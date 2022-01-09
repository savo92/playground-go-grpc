package server

import (
	"fmt"
	"sync"
)

type roomManager struct {
	rooms map[roomID]*room
	mu    sync.Mutex

	closed bool
}

func (rm *roomManager) createRoom() (roomID, error) {
	if rm.closed {
		return roomID(""), fmt.Errorf("room manager already closed")
	}
	r, err := newRoom("default")
	if err != nil {
		return roomID(""), err
	}
	r.rm = rm
	rm.mu.Lock()
	rm.rooms[r.id] = r
	rm.mu.Unlock()

	return r.id, nil
}

func (rm *roomManager) removeRoom(id roomID) {
	rm.mu.Lock()
	delete(rm.rooms, id)
	rm.mu.Unlock()
}

func (rm *roomManager) close() {
	if rm.closed {
		return
	}
	rm.closed = true
	for _, r := range rm.rooms {
		r.close()
	}
}

func newRoomManager() (*roomManager, error) {
	rm := &roomManager{
		rooms: make(map[roomID]*room),
	}

	return rm, nil
}
