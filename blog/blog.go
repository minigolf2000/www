package blog

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/boltdb/bolt"
	"github.com/russross/blackfriday"
)

var (
	db    *bolt.DB
	posts []Post
)

var (
	bucketPosts = []byte("posts")
)

// Post represents an individual blog entry.
type Post struct {
	ID       string
	Markdown []byte
	HTML     []byte
	Metadata
}

// Metadata represents metadata about a post. It's stored in boltdb, instead
// of on the file system with the markdown.
type Metadata struct {
	Hash      [32]byte
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Posts returns all of the posts in the blog.
func Posts() []Post {
	return posts
}

// Load takes a directory and loads all of the markdown files in the
// directory as posts.
func Load(dir string) error {
	var err error
	db, err = bolt.Open("blog.db", 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return err
	}

	// Create the posts bucket if it doesn't already exist.
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketPosts)
		return err
	})
	if err != nil {
		return err
	}

	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if filepath.Ext(path) != ".md" {
			return nil
		}
		b, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}

		name := filepath.Base(path)
		name = strings.TrimSuffix(name, ".md")

		p := Post{
			ID:       name,
			Markdown: b,
			HTML:     blackfriday.MarkdownCommon(b),
			Metadata: Metadata{},
		}
		markdownHash := sha256.New().Sum(b)
		copy(p.Metadata.Hash[:], markdownHash)

		err = lookupMetadata(&p)
		if err != nil {
			return err
		}

		posts = append(posts, p)
		return nil
	})
	return err
}

func lookupMetadata(p *Post) error {
	err := db.Batch(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketPosts)
		v := b.Get([]byte(p.ID))

		var (
			m   Metadata
			err error
		)
		if v != nil {
			decoder := gob.NewDecoder(bytes.NewReader(v))
			err = decoder.Decode(&m)
			if err != nil {
				return err
			}
		}

		// We need to update the metadata and do a Put.
		if !bytes.Equal(p.Hash[:], m.Hash[:]) {
			m.Hash = p.Hash
			if m.CreatedAt.IsZero() {
				m.CreatedAt = time.Now()
			}
			m.UpdatedAt = time.Now()

			var buf bytes.Buffer
			encoder := gob.NewEncoder(&buf)
			err = encoder.Encode(m)
			if err != nil {
				return err
			}

			err = b.Put([]byte(p.ID), buf.Bytes())
			if err != nil {
				return err
			}
			p.Metadata = m
		}
		return err
	})
	return err
}
