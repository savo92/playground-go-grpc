package server

import (
	"fmt"
	"sync"

	log "github.com/sirupsen/logrus"
)

type RoomManager struct {
	rooms map[RoomID]*room
	mu    sync.Mutex

	closed bool
}

func (rm *RoomManager) CreateRoom(name string) (RoomID, error) {
	if rm.closed {
		return RoomID(""), fmt.Errorf("room manager already closed")
	}
	r, err := newRoom(name)
	if err != nil {
		return RoomID(""), err
	}
	r.rm = rm
	rm.mu.Lock()
	rm.rooms[r.id] = r
	rm.mu.Unlock()

	return r.id, nil
}

func (rm *RoomManager) GetRoom(id RoomID) (*room, bool) {
	r, ok := rm.rooms[id]

	return r, ok
}

func (rm *RoomManager) removeRoom(id RoomID) {
	log.Debugf("Room %s removed from manager", rm.rooms[id].name)
	rm.mu.Lock()
	delete(rm.rooms, id)
	rm.mu.Unlock()
}

func (rm *RoomManager) Close() {
	if rm.closed {
		return
	}
	log.Debugf("Closing room manager")
	rm.closed = true
	for _, r := range rm.rooms {
		r.close()
	}
}

func NewRoomManager() (*RoomManager, error) {
	rm := &RoomManager{
		rooms: make(map[RoomID]*room),
	}

	return rm, nil
}
