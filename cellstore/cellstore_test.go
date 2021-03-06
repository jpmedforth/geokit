package cellstore

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/golang/geo/s2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "geokit/cellstore")
}

func BenchmarkReader(b *testing.B) {
	const minID = s2.CellID(1317624576693539401)
	var value = []byte("testdatatestdata")

	runBench := func(b *testing.B, numRecords int, compression Compression) {
		f, err := ioutil.TempFile("", "cellstore-bench")
		if err != nil {
			b.Fatal(err)
		}
		defer os.Remove(f.Name())
		defer f.Close()

		w := NewWriter(f, &Options{Compression: compression})
		defer w.Close()

		for i := 0; i < 8*numRecords; i += 8 {
			if err := w.Append(minID+s2.CellID(i), value); err != nil {
				b.Fatal(err)
			}
		}
		if err := w.Close(); err != nil {
			b.Fatal(err)
		}
		if err := f.Close(); err != nil {
			b.Fatal(err)
		}

		if f, err = os.Open(f.Name()); err != nil {
			b.Fatal(err)
		}
		defer f.Close()

		fi, err := os.Stat(f.Name())
		if err != nil {
			b.Fatal(err)
		}

		r, err := NewReader(f, fi.Size())
		if err != nil {
			b.Fatal(err)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cellID := minID + s2.CellID((i%numRecords)*8)

			it, err := r.FindBlock(cellID)
			if err != nil {
				b.Fatalf("error finding cell %d: %v", cellID, err)
			}
			if !it.Next() {
				b.Fatalf("expected to be able to advance to next entry")
			}
			it.Release()
		}
	}

	b.Run("1k uncompressed", func(b *testing.B) {
		runBench(b, 1000, NoCompression)
	})
	b.Run("10M uncompressed", func(b *testing.B) {
		runBench(b, 10*1000*1000, NoCompression)
	})
	b.Run("1k snappy", func(b *testing.B) {
		runBench(b, 1000, SnappyCompression)
	})
	b.Run("10M snappy", func(b *testing.B) {
		runBench(b, 10*1000*1000, SnappyCompression)
	})
}
