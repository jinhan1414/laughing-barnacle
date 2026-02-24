package memory

import (
	"encoding/json"
	"fmt"
	bolt "go.etcd.io/bbolt"
	"sort"
	"strings"
)

func (s *Store) readNodeLocked(path string) (Node, bool, error) {
	var node Node
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketNodes))
		if b == nil {
			return nil
		}
		raw := b.Get([]byte(path))
		if raw == nil {
			return nil
		}
		return json.Unmarshal(raw, &node)
	})
	if err != nil {
		return Node{}, false, err
	}
	if strings.TrimSpace(node.Path) == "" {
		return Node{}, false, nil
	}
	return node, true, nil
}

func (s *Store) writeNodeLocked(node Node) error {
	data, err := json.Marshal(node)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketNodes))
		if b == nil {
			return fmt.Errorf("memory nodes bucket missing")
		}
		return b.Put([]byte(node.Path), data)
	})
}

func (s *Store) deleteNodeKeyLocked(path string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketNodes))
		if b == nil {
			return nil
		}
		return b.Delete([]byte(path))
	})
}

func (s *Store) readChildrenLocked(path string) ([]string, error) {
	var out []string
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketChildren))
		if b == nil {
			return nil
		}
		raw := b.Get([]byte(path))
		if raw == nil {
			return nil
		}
		return json.Unmarshal(raw, &out)
	})
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	sort.Strings(out)
	return out, nil
}

func (s *Store) writeChildrenLocked(path string, children []string) error {
	children = normalizePathList(children)
	data, err := json.Marshal(children)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketChildren))
		if b == nil {
			return fmt.Errorf("memory children bucket missing")
		}
		return b.Put([]byte(path), data)
	})
}

func (s *Store) deleteChildrenKeyLocked(path string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketChildren))
		if b == nil {
			return nil
		}
		return b.Delete([]byte(path))
	})
}

func (s *Store) addChildLocked(dirPath, childPath string) error {
	children, err := s.readChildrenLocked(dirPath)
	if err != nil {
		return err
	}
	children = append(children, childPath)
	return s.writeChildrenLocked(dirPath, children)
}

func (s *Store) removeChildLocked(dirPath, childPath string) error {
	children, err := s.readChildrenLocked(dirPath)
	if err != nil {
		return err
	}
	out := make([]string, 0, len(children))
	for _, path := range children {
		if path != childPath {
			out = append(out, path)
		}
	}
	return s.writeChildrenLocked(dirPath, out)
}

func (s *Store) readSegmentLocked(id string) (Segment, bool, error) {
	var seg Segment
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketSegments))
		if b == nil {
			return nil
		}
		raw := b.Get([]byte(id))
		if raw == nil {
			return nil
		}
		return json.Unmarshal(raw, &seg)
	})
	if err != nil {
		return Segment{}, false, err
	}
	if strings.TrimSpace(seg.ID) == "" {
		return Segment{}, false, nil
	}
	return seg, true, nil
}

func (s *Store) writeSegmentLocked(seg Segment) error {
	data, err := json.Marshal(seg)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketSegments))
		if b == nil {
			return fmt.Errorf("memory segments bucket missing")
		}
		return b.Put([]byte(seg.ID), data)
	})
}

func (s *Store) listSegmentsLocked() ([]Segment, error) {
	out := make([]Segment, 0, 16)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketSegments))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var seg Segment
			if err := json.Unmarshal(v, &seg); err != nil {
				return nil
			}
			out = append(out, seg)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.Before(out[j].UpdatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *Store) readMetaStringLocked(key string) (string, error) {
	var value string
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketMeta))
		if b == nil {
			return nil
		}
		value = strings.TrimSpace(string(b.Get([]byte(key))))
		return nil
	})
	return value, err
}

func (s *Store) writeMetaStringLocked(key, value string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketMeta))
		if b == nil {
			return fmt.Errorf("memory meta bucket missing")
		}
		return b.Put([]byte(key), []byte(strings.TrimSpace(value)))
	})
}
