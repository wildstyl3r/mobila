package main

import "fmt"

type State struct {
	Node     *Libp2pNode
	Store    *Store
	Contacts []Contact
}

func NewState() *State {
	return &State{}
}
func (s *State) Shutdown() {
	if s.Node != nil {
		s.Node.Shutdown()
	}
	if s.Store != nil {
		s.Store.Close()
	}
}

func (s *State) LoadContacts() (err error) {
	s.Contacts, err = s.Store.GetAllContacts()
	return
}

func (s *State) AddContact(c Contact) {
	for i := range s.Contacts {
		if s.Contacts[i].ID == c.ID {
			fmt.Printf("contact already exists: %s\n", c.Alias)
			return
		}
	}
	fmt.Printf("add contact: %s\n", c.Alias)

	s.Contacts = append(s.Contacts, c)
	s.Store.AddContact(c)
}
