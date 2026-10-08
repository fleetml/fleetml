// Package delta makes and applies binary deltas between two versions of a
// model file, so a device that has the previous version downloads only what
// changed.
//
// The diff is the bsdiff algorithm (suffix sorting with qsufsort, then
// approximate matches with byte-wise differences). Its control, diff and
// extra sections are stored raw-DEFLATE compressed instead of bzip2, so the
// Android and iOS SDKs can apply a delta with their platform's inflater. The
// format is not BSDIFF40-compatible:
//
//	"FMLDLT01"                              8 bytes
//	new size, ctrl size, diff size          3 x int64, little endian (sizes compressed)
//	ctrl: raw DEFLATE of int64 triples (add, copy, seek), little endian
//	diff: raw DEFLATE of the bytes added to old bytes
//	extra: raw DEFLATE of the bytes copied as they are
//
// To apply: for each triple, out += old[pos:pos+add] + diff bytes (bytewise,
// mod 256); pos += add; out += next copy bytes of extra; pos += seek.
//
// The diff algorithm is derived from bsdiff by Colin Percival:
//
//	Copyright 2003-2005 Colin Percival. All rights reserved.
//
//	Redistribution and use in source and binary forms, with or without
//	modification, are permitted providing that the following conditions
//	are met:
//	1. Redistributions of source code must retain the above copyright
//	   notice, this list of conditions and the following disclaimer.
//	2. Redistributions in binary form must reproduce the above copyright
//	   notice, this list of conditions and the following disclaimer in the
//	   documentation and/or other materials provided with the distribution.
//
//	THIS SOFTWARE IS PROVIDED BY THE AUTHOR ``AS IS'' AND ANY EXPRESS OR
//	IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
//	WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
//	ARE DISCLAIMED. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY
//	DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
//	DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS
//	OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION)
//	HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT,
//	STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING
//	IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
//	POSSIBILITY OF SUCH DAMAGE.
package delta

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// Magic starts every delta.
const Magic = "FMLDLT01"

const headerSize = len(Magic) + 3*8

// MaxSize bounds the files Diff accepts (its suffix arrays use int32).
const MaxSize = math.MaxInt32 - 1

// Diff returns a delta that turns old into next.
func Diff(old, next []byte) ([]byte, error) {
	if len(old) > MaxSize || len(next) > MaxSize {
		return nil, fmt.Errorf("delta: files above %d bytes are not supported", MaxSize)
	}
	ctrl, diff, extra := bsdiff(old, next)

	var ctrlBuf bytes.Buffer
	for _, c := range ctrl {
		_ = binary.Write(&ctrlBuf, binary.LittleEndian, c)
	}
	cCtrl, err := deflate(ctrlBuf.Bytes())
	if err != nil {
		return nil, err
	}
	cDiff, err := deflate(diff)
	if err != nil {
		return nil, err
	}
	cExtra, err := deflate(extra)
	if err != nil {
		return nil, err
	}
	out := bytes.NewBuffer(make([]byte, 0, headerSize+len(cCtrl)+len(cDiff)+len(cExtra)))
	out.WriteString(Magic)
	_ = binary.Write(out, binary.LittleEndian, [3]int64{int64(len(next)), int64(len(cCtrl)), int64(len(cDiff))})
	out.Write(cCtrl)
	out.Write(cDiff)
	out.Write(cExtra)
	return out.Bytes(), nil
}

// Patch applies a delta made by Diff to old.
func Patch(old, patch []byte) ([]byte, error) {
	if len(patch) < headerSize || string(patch[:len(Magic)]) != Magic {
		return nil, errors.New("delta: not a FleetML delta")
	}
	var hdr [3]int64
	_ = binary.Read(bytes.NewReader(patch[len(Magic):headerSize]), binary.LittleEndian, &hdr)
	newSize, ctrlLen, diffLen := hdr[0], hdr[1], hdr[2]
	rest := int64(len(patch) - headerSize)
	if newSize < 0 || newSize > MaxSize || ctrlLen < 0 || diffLen < 0 || ctrlLen+diffLen > rest {
		return nil, errors.New("delta: corrupt header")
	}
	body := patch[headerSize:]
	ctrl := flate.NewReader(bytes.NewReader(body[:ctrlLen]))
	diff := flate.NewReader(bytes.NewReader(body[ctrlLen : ctrlLen+diffLen]))
	extra := flate.NewReader(bytes.NewReader(body[ctrlLen+diffLen:]))
	defer func() { _ = ctrl.Close(); _ = diff.Close(); _ = extra.Close() }()

	out := make([]byte, newSize)
	var oldPos, newPos int64
	for newPos < newSize {
		var c [3]int64
		if err := binary.Read(ctrl, binary.LittleEndian, &c); err != nil {
			return nil, fmt.Errorf("delta: reading control: %w", err)
		}
		add, cp, seek := c[0], c[1], c[2]
		if add < 0 || cp < 0 || newPos+add+cp > newSize {
			return nil, errors.New("delta: corrupt control")
		}
		if _, err := io.ReadFull(diff, out[newPos:newPos+add]); err != nil {
			return nil, fmt.Errorf("delta: reading diff: %w", err)
		}
		for i := int64(0); i < add; i++ {
			if p := oldPos + i; p >= 0 && p < int64(len(old)) {
				out[newPos+i] += old[p]
			}
		}
		newPos += add
		oldPos += add
		if _, err := io.ReadFull(extra, out[newPos:newPos+cp]); err != nil {
			return nil, fmt.Errorf("delta: reading extra: %w", err)
		}
		newPos += cp
		oldPos += seek
	}
	return out, nil
}

func deflate(b []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(b); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// bsdiff computes the control triples and the diff and extra bytes.
func bsdiff(old, next []byte) (ctrl [][3]int64, diff, extra []byte) {
	oldsize, newsize := len(old), len(next)
	sa := qsufsort(old)

	var scan, length, lastscan, lastpos, lastoffset int
	var pos int
	for scan < newsize {
		oldscore := 0
		scan += length
		for scsc := scan; scan < newsize; scan++ {
			pos, length = search(sa, old, next[scan:], 0, oldsize)
			for ; scsc < scan+length; scsc++ {
				if scsc+lastoffset < oldsize && old[scsc+lastoffset] == next[scsc] {
					oldscore++
				}
			}
			if (length == oldscore && length != 0) || length > oldscore+8 {
				break
			}
			if scan+lastoffset < oldsize && old[scan+lastoffset] == next[scan] {
				oldscore--
			}
		}

		if length != oldscore || scan == newsize {
			s, Sf, lenf := 0, 0, 0
			for i := 0; lastscan+i < scan && lastpos+i < oldsize; {
				if old[lastpos+i] == next[lastscan+i] {
					s++
				}
				i++
				if s*2-i > Sf*2-lenf {
					Sf, lenf = s, i
				}
			}

			lenb := 0
			if scan < newsize {
				s, Sb := 0, 0
				for i := 1; scan >= lastscan+i && pos >= i; i++ {
					if old[pos-i] == next[scan-i] {
						s++
					}
					if s*2-i > Sb*2-lenb {
						Sb, lenb = s, i
					}
				}
			}

			if lastscan+lenf > scan-lenb {
				overlap := (lastscan + lenf) - (scan - lenb)
				s, Ss, lens := 0, 0, 0
				for i := 0; i < overlap; i++ {
					if next[lastscan+lenf-overlap+i] == old[lastpos+lenf-overlap+i] {
						s++
					}
					if next[scan-lenb+i] == old[pos-lenb+i] {
						s--
					}
					if s > Ss {
						Ss, lens = s, i+1
					}
				}
				lenf += lens - overlap
				lenb -= lens
			}

			for i := 0; i < lenf; i++ {
				diff = append(diff, next[lastscan+i]-old[lastpos+i])
			}
			extra = append(extra, next[lastscan+lenf:scan-lenb]...)
			ctrl = append(ctrl, [3]int64{int64(lenf), int64((scan - lenb) - (lastscan + lenf)), int64((pos - lenb) - (lastpos + lenf))})

			lastscan = scan - lenb
			lastpos = pos - lenb
			lastoffset = pos - scan
		}
	}
	return ctrl, diff, extra
}

func matchlen(a, b []byte) int {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	return i
}

// search finds the longest match of target in old among the suffixes sa[st..en].
func search(sa []int32, old, target []byte, st, en int) (pos, length int) {
	for en-st >= 2 {
		x := st + (en-st)/2
		if bytes.Compare(old[sa[x]:], target) < 0 {
			st = x
		} else {
			en = x
		}
	}
	x := matchlen(old[sa[st]:], target)
	y := matchlen(old[sa[en]:], target)
	if x > y {
		return int(sa[st]), x
	}
	return int(sa[en]), y
}

// qsufsort builds the suffix array of old (Larsson and Sadakane's algorithm).
func qsufsort(old []byte) []int32 {
	n := len(old)
	sa := make([]int32, n+1)
	rank := make([]int32, n+1)
	var buckets [256]int32
	for _, b := range old {
		buckets[b]++
	}
	for i := 1; i < 256; i++ {
		buckets[i] += buckets[i-1]
	}
	for i := 255; i > 0; i-- {
		buckets[i] = buckets[i-1]
	}
	buckets[0] = 0
	for i, b := range old {
		buckets[b]++
		sa[buckets[b]] = int32(i)
	}
	sa[0] = int32(n)
	for i, b := range old {
		rank[i] = buckets[b]
	}
	rank[n] = 0
	for i := 1; i < 256; i++ {
		if buckets[i] == buckets[i-1]+1 {
			sa[buckets[i]] = -1
		}
	}
	sa[0] = -1

	for h := 1; sa[0] != -int32(n+1); h += h {
		length := 0
		i := 0
		for i < n+1 {
			if sa[i] < 0 {
				length -= int(sa[i])
				i -= int(sa[i])
			} else {
				if length != 0 {
					sa[i-length] = -int32(length)
				}
				length = int(rank[sa[i]]) + 1 - i
				split(sa, rank, i, length, h)
				i += length
				length = 0
			}
		}
		if length != 0 {
			sa[i-length] = -int32(length)
		}
	}
	for i := 0; i < n+1; i++ {
		sa[rank[i]] = int32(i)
	}
	return sa
}

func split(sa, rank []int32, start, length, h int) {
	if length < 16 {
		for k := start; k < start+length; {
			j := 1
			x := rank[int(sa[k])+h]
			for i := 1; k+i < start+length; i++ {
				if rank[int(sa[k+i])+h] < x {
					x = rank[int(sa[k+i])+h]
					j = 0
				}
				if rank[int(sa[k+i])+h] == x {
					sa[k+j], sa[k+i] = sa[k+i], sa[k+j]
					j++
				}
			}
			for i := 0; i < j; i++ {
				rank[sa[k+i]] = int32(k + j - 1)
			}
			if j == 1 {
				sa[k] = -1
			}
			k += j
		}
		return
	}

	x := rank[int(sa[start+length/2])+h]
	jj, kk := 0, 0
	for i := start; i < start+length; i++ {
		if rank[int(sa[i])+h] < x {
			jj++
		}
		if rank[int(sa[i])+h] == x {
			kk++
		}
	}
	jj += start
	kk += jj

	i, j, k := start, 0, 0
	for i < jj {
		switch {
		case rank[int(sa[i])+h] < x:
			i++
		case rank[int(sa[i])+h] == x:
			sa[i], sa[jj+j] = sa[jj+j], sa[i]
			j++
		default:
			sa[i], sa[kk+k] = sa[kk+k], sa[i]
			k++
		}
	}
	for jj+j < kk {
		if rank[int(sa[jj+j])+h] == x {
			j++
		} else {
			sa[jj+j], sa[kk+k] = sa[kk+k], sa[jj+j]
			k++
		}
	}

	if jj > start {
		split(sa, rank, start, jj-start, h)
	}
	for i := 0; i < kk-jj; i++ {
		rank[sa[jj+i]] = int32(kk - 1)
	}
	if jj == kk-1 {
		sa[jj] = -1
	}
	if start+length > kk {
		split(sa, rank, kk, start+length-kk, h)
	}
}
