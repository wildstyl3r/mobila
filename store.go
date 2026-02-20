package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/crypto/argon2"
)

type Store struct {
	DB   *badger.DB
	Salt []byte
}

var appName = "mobila"

func NewStore(password string) (*Store, error) {
	var err error
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	fullPath := filepath.Join(configDir, appName)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		err := os.MkdirAll(fullPath, 0700)
		if err != nil {
			return nil, err
		}
	}

	fmt.Printf("storage at %s \n", fullPath+"/store")
	opts := badger.DefaultOptions(fullPath + "/store")
	salt := []byte("constant_salt_for_app")

	if password != "" {

		key := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
		opts.EncryptionKey = key
		opts.IndexCacheSize = 100 << 20
	}

	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}
	return &Store{DB: db, Salt: salt}, nil
}

func (s *Store) Close() {
	s.DB.Close()
}

func (s *Store) SaveBootstrapPeer(info peer.AddrInfo) error {
	return s.DB.Update(func(txn *badger.Txn) error {
		key := []byte("boot:" + info.ID.String())
		val, err := json.Marshal(info)
		if err != nil {
			return err
		}
		return txn.Set(key, val)
	})
}

func (s *Store) LoadBootstrapPeers() ([]peer.AddrInfo, error) {
	var peers []peer.AddrInfo
	err := s.DB.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("boot:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			err := it.Item().Value(func(v []byte) error {
				var info peer.AddrInfo
				if err := json.Unmarshal(v, &info); err != nil {
					return err
				}
				peers = append(peers, info)
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	return peers, err
}

const nodeKeyPath = "system:node_key"

func (s *Store) SavePrivateKey(priv crypto.PrivKey) error {
	return s.DB.Update(func(txn *badger.Txn) error {
		data, err := crypto.MarshalPrivateKey(priv)
		if err != nil {
			return err
		}
		return txn.Set([]byte(nodeKeyPath), data)
	})
}

func (s *Store) LoadPrivateKey() (crypto.PrivKey, error) {
	var priv crypto.PrivKey
	err := s.DB.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(nodeKeyPath))
		if err != nil {
			return err
		}
		return item.Value(func(v []byte) error {
			var err error
			priv, err = crypto.UnmarshalPrivateKey(v)
			return err
		})
	})
	return priv, err
}

type Contact struct {
	ID        string   `json:"id"`
	Alias     string   `json:"alias"`
	Addresses []string `json:"addresses"`
	LastSeen  int64    `json:"last_seen"`
}

func (s *Store) AddContact(c Contact) error {
	err := s.DB.Update(func(txn *badger.Txn) error {
		key := []byte("contact:" + c.ID)
		val, err := json.Marshal(c)
		if err != nil {
			return err
		}
		return txn.Set(key, val)
	})
	if err != nil {
		return err
	}

	return err
}

func (s *Store) GetAllContacts() (map[string]Contact, error) {
	contacts := make(map[string]Contact)
	err := s.DB.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("contact:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			err := item.Value(func(v []byte) error {
				var c Contact
				if err := json.Unmarshal(v, &c); err != nil {
					return err
				}
				contacts[c.ID] = c
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	return contacts, err
}

func (s *Store) GetChatList() ([]Chat, error) {
	var chats []Chat
	err := s.DB.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte("chat:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var c Chat
			_ = it.Item().Value(func(v []byte) error {
				return json.Unmarshal(v, &c)
			})
			chats = append(chats, c)
		}
		return nil
	})
	return chats, err
}

func (s *Store) getChatMemberIDs(txn *badger.Txn, chatID string) ([]string, error) {
	var peerIDs []string
	prefix := []byte("member:" + chatID + ":")

	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = false

	it := txn.NewIterator(opts)
	defer it.Close()

	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		key := it.Item().Key()
		peerID := string(key[len(prefix):])
		peerIDs = append(peerIDs, peerID)
	}

	return peerIDs, nil
}

func (s *Store) getMessagesForChat(txn *badger.Txn, chatID string) ([]Message, error) {
	var msgs []Message
	prefix := []byte("msg:" + chatID + ":")

	it := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()

	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		item := it.Item()

		var m Message
		err := item.Value(func(v []byte) error {
			return json.Unmarshal(v, &m)
		})
		if err != nil {
			fmt.Printf("error seeking messages: %v", err)
			continue
		}

		m.ChatID = chatID

		key := item.Key()
		keysuf := strings.Split(string(key[len(prefix):]), ":")
		m.ID = keysuf[1]
		nsec, err := strconv.Atoi(keysuf[0])
		if err != nil {
			fmt.Printf("unable to convert key timestamp to nanoseconds: %v", err)
		} else {
			m.Sent = time.Unix(0, int64(nsec))
		}

		msgs = append(msgs, m)
	}

	return msgs, nil
}

func (s *Store) createChatHeader(c Chat) error {
	return s.DB.Update(func(txn *badger.Txn) error {
		key := []byte("chat:" + c.ID)
		data, _ := json.Marshal(c)
		return txn.Set(key, data)
	})
}

func (s *Store) AddChatMember(chatID string, peerID string) error {
	return s.DB.Update(func(txn *badger.Txn) error {
		key := fmt.Appendf(nil, "member:%s:%s", chatID, peerID)
		return txn.Set(key, []byte{})
	})
}

func (s *Store) CreateNewChat(c Chat) error {
	err := s.createChatHeader(c)
	if err != nil {
		return err
	}

	for _, pID := range c.Peers {
		s.AddChatMember(c.ID, pID)
	}
	return nil
}

func (s *Store) GetFullChat(chat *Chat, allContacts map[string]Contact) error {
	err := s.DB.View(func(txn *badger.Txn) error {
		item, _ := txn.Get([]byte("chat:" + chat.ID))
		item.Value(func(v []byte) error { return json.Unmarshal(v, chat) })
		chat.Peers, _ = s.getChatMemberIDs(txn, chat.ID)
		chat.Messages, _ = s.getMessagesForChat(txn, chat.ID)

		return nil
	})
	return err
}

func (s *Store) AddMessage(m Message) error {
	return s.DB.Update(func(txn *badger.Txn) error {
		ts := m.Sent.UnixNano()
		key := fmt.Appendf(nil, "msg:%s:%020d:%s", m.ChatID, ts, m.ID)

		data, _ := json.Marshal(m)
		return txn.Set(key, data)
	})
}
