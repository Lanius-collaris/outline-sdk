// Copyright 2023 The Outline Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tlsrecordfrag

import (
	"encoding/binary"
	"io"
)

type tlsRecordFragWriter struct {
	writer      io.Writer
	prefixBytes int32
}

// Record Layer: https://datatracker.ietf.org/doc/html/rfc8446#section-5.1
const maxRecordLength = 1 << 14 //For the fragments, not for the reassembled record
const typeHandshake = 22

func NewWriter(writer io.Writer, prefixBytes int32) *tlsRecordFragWriter {
	return &tlsRecordFragWriter{writer, prefixBytes}
}

func (w *tlsRecordFragWriter) dontFrag(first []byte, source io.Reader) (written int64, err error) {
	tmp, err := w.writer.Write(first)
	written = int64(tmp)
	w.prefixBytes = 0
	if err != nil {
		return written, err
	}
	n, err := io.Copy(w.writer, source)
	written += n
	return written, err
}

func (w *tlsRecordFragWriter) ReadFrom(source io.Reader) (written int64, err error) {
	if 0 < w.prefixBytes {
		var recordHeader [5]byte
		_, err := io.ReadFull(source, recordHeader[:])
		if err != nil {
			return 0, err
		}
		recordLength := int32(binary.BigEndian.Uint16(recordHeader[3:]))
		if recordHeader[0] != typeHandshake || w.prefixBytes >= recordLength {
			return w.dontFrag(recordHeader[:], source)
		}
		if recordLength > maxRecordLength {
			return w.dontFrag(recordHeader[:], source)
		}
		// Allocate buffer that fits the entire record after the split (2*header + payload).
		buf := make([]byte, recordLength+10)
		header := recordHeader[:3]

		copy(buf, header)
		binary.BigEndian.PutUint16(buf[3:], uint16(w.prefixBytes))
		n2, err := io.ReadFull(source, buf[5:5+w.prefixBytes])
		if err != nil {
			w.prefixBytes = 0
			return 0, err
		}

		copy(buf[5+n2:], header)
		binary.BigEndian.PutUint16(buf[5+n2+3:], uint16(recordLength-w.prefixBytes))
		_, err = io.ReadFull(source, buf[10+w.prefixBytes:])
		if err != nil {
			w.prefixBytes = 0
			return 0, err
		}

		tmp, err := w.writer.Write(buf)
		w.prefixBytes = 0
		if tmp >= 5 { //subtract bytes of added header
			tmp -= 5
		}
		written = int64(tmp)
		if err != nil {
			return written, err
		}
	}
	n, err := io.Copy(w.writer, source)
	written += n
	return written, err
}

func (w *tlsRecordFragWriter) Write(data []byte) (written int, err error) {
	if 0 < w.prefixBytes {
		length := int32(len(data))
		if w.prefixBytes+5 >= length {
			w.prefixBytes = 0
			return w.writer.Write(data)
		}

		recordLength := int32(binary.BigEndian.Uint16(data[3:]))
		remainder := data[5:]
		remainderLength := length - 5
		hasPartialRecord := recordLength > remainderLength
		hasMultipleRecords := recordLength < remainderLength
		isRecordOverflow := recordLength > maxRecordLength

		if data[0] != typeHandshake || hasPartialRecord || w.prefixBytes == recordLength || isRecordOverflow {
			w.prefixBytes = 0
			return w.writer.Write(data)
		}
		//Need fragmentation, allocate data + header*1
		buf := make([]byte, length+5)
		header := data[:3]
		record1 := remainder[:w.prefixBytes]
		record2 := remainder[w.prefixBytes:recordLength]

		copy(buf, header)
		binary.BigEndian.PutUint16(buf[3:], uint16(w.prefixBytes))
		copy(buf[5:], record1)

		copy(buf[5+w.prefixBytes:], header)
		binary.BigEndian.PutUint16(buf[5+3+w.prefixBytes:], uint16(len(record2)))
		copy(buf[5+5+w.prefixBytes:], record2)

		if hasMultipleRecords {
			copy(buf[5+5+recordLength:], remainder[recordLength:])
		}

		w.prefixBytes = 0
		written, err = w.writer.Write(buf)
		if written >= 5 {
			written -= 5
		}
		return written, err
	}
	return w.writer.Write(data)
}
