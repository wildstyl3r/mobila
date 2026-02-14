package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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
	ID        string   `json:"-"`
	Alias     string   `json:"alias"`
	Addresses []string `json:"addresses"`
	LastSeen  int64    `json:"last_seen"`
}

func (s *Store) AddContact(c Contact) error {
	return s.DB.Update(func(txn *badger.Txn) error {
		key := []byte("contact:" + c.ID)
		val, err := json.Marshal(c)
		if err != nil {
			return err
		}
		return txn.Set(key, val)
	})
}

func (s *Store) GetAllContacts() ([]Contact, error) {
	var contacts []Contact
	err := s.DB.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("contact:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			idFromKey := string(item.Key()[len(prefix):])
			err := item.Value(func(v []byte) error {
				var c Contact
				if err := json.Unmarshal(v, &c); err != nil {
					return err
				}
				c.ID = idFromKey
				contacts = append(contacts, c)
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
