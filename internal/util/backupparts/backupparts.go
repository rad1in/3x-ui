// Package backupparts fits a backup under a chat app's upload cap.
package backupparts

import (
	"bytes"
	"compress/gzip"
	"fmt"
)

// Part is one file of a backup as it is uploaded.
type Part struct {
	Name string
	Data []byte
}

// Split returns the backup whole when it fits limit, else gzipped, and cut into
// name.gz.partNN files when even that is too big: `cat name.gz.part* | gunzip`.
func Split(name string, data []byte, limit int) ([]Part, error) {
	if len(data) <= limit {
		return []Part{{Name: name, Data: data}}, nil
	}
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := zw.Write(data); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	gz := buf.Bytes()
	if len(gz) <= limit {
		return []Part{{Name: name + ".gz", Data: gz}}, nil
	}
	count := (len(gz) + limit - 1) / limit
	width := max(2, len(fmt.Sprint(count)))
	parts := make([]Part, 0, count)
	for i := 0; i < count; i++ {
		end := min((i+1)*limit, len(gz))
		parts = append(parts, Part{Name: fmt.Sprintf("%s.gz.part%0*d", name, width, i+1), Data: gz[i*limit : end]})
	}
	return parts, nil
}
