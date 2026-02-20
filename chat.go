package main

import "time"

type Message struct {
	ID     string    `json:"id"`
	Prev   string    `json:"prev"`
	ChatID string    `json:"chat_id"`
	Author string    `json:"author_id"`
	Text   string    `json:"content"`
	Sent   time.Time `json:"sent"`
}

type Chat struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Messages []Message `json:"-"`
	Peers    []string  `json:"-"`
}

func (c *Chat) GetMessage(mID string) *Message {
	for i := range c.Messages {
		if c.Messages[i].ID == mID {
			return &c.Messages[i]
		}
	}
	return nil
}
