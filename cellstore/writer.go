package cellstore

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/golang/geo/s2"
	"github.com/golang/snappy"
)

// Writer represents a cellstore Writer
type Writer struct {
	w io.Writer
	o Options

	last   s2.CellID // the last cellID
	offset int64     // the current offset

	buf   []byte
	snp   []byte
	tmp   []byte
	index []blockInfo
}

// NewWriter wraps a writer and returns a cellstore Writer
func NewWriter(w io.Writer, o *Options) *Writer {
	var opts Options
	if o != nil {
		opts = *o
	}
	opts.norm()

	return &Writer{
		w:   w,
		o:   opts,
		tmp: make([]byte, 2*binary.MaxVarintLen64),
	}
}

// Append appends a cell to the store.
func (w *Writer) Append(cellID s2.CellID, data []byte) error {
	if w.tmp == nil {
		return errClosed
	}
	if !cellID.IsValid() {
		return errInvalidCellID
	} else if w.last >= cellID {
		return fmt.Errorf("cellstore: attempted an out-of-order append, %v must be > %v", cellID, w.last)
	}

	if len(w.buf) != 0 && len(w.buf)+len(data)+2*binary.MaxVarintLen64 > w.o.BlockSize {
		if err := w.flush(); err != nil {
			return err
		}
	}

	key := cellID
	if len(w.buf) != 0 { // delta-encode CellID
		key -= w.last
	}
	n := binary.PutUvarint(w.tmp[0:], uint64(key))
	n += binary.PutUvarint(w.tmp[n:], uint64(len(data)))

	w.buf = append(w.buf, w.tmp[:n]...)
	w.buf = append(w.buf, data...)
	w.last = cellID
	return nil
}

// Close closes the writer
func (w *Writer) Close() error {
	if w.tmp == nil {
		return errClosed
	}
	if err := w.flush(); err != nil {
		return err
	}

	indexOffset := w.offset
	if err := w.writeIndex(); err != nil {
		return err
	}

	if err := w.writeFooter(indexOffset); err != nil {
		return err
	}
	w.tmp = nil
	return nil
}

func (w *Writer) writeIndex() error {
	var last blockInfo
	for i, ent := range w.index {
		cid, off := ent.MaxCellID, ent.Offset
		if i > 0 { // delta-encode
			cid -= last.MaxCellID
			off -= last.Offset
		}
		last = ent

		n := binary.PutUvarint(w.tmp[0:], uint64(cid))
		n += binary.PutUvarint(w.tmp[n:], uint64(off))
		if err := w.writeRaw(w.tmp[:n]); err != nil {
			return err
		}
	}
	return nil
}

func (w *Writer) writeFooter(indexOffset int64) error {
	binary.LittleEndian.PutUint64(w.tmp[0:], uint64(indexOffset))
	if err := w.writeRaw(w.tmp[:8]); err != nil {
		return err
	}
	if err := w.writeRaw(magic); err != nil {
		return err
	}
	return nil
}

func (w *Writer) writeRaw(p []byte) error {
	n, err := w.w.Write(p)
	w.offset += int64(n)
	return err
}

func (w *Writer) flush() error {
	if len(w.buf) == 0 {
		return nil
	}

	w.index = append(w.index, blockInfo{
		MaxCellID: w.last,
		Offset:    w.offset,
	})

	var block []byte
	switch w.o.Compression {
	case SnappyCompression:
		w.snp = snappy.Encode(w.snp[:cap(w.snp)], w.buf)
		if len(w.snp) < len(w.buf)-len(w.buf)/8 {
			block = append(w.snp, blockSnappyCompression)
		} else {
			block = append(w.buf, blockNoCompression)
		}
	default:
		block = append(w.buf, blockNoCompression)
	}

	w.buf = w.buf[:0]
	return w.writeRaw(block)
}
