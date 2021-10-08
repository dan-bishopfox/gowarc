/*
 * Copyright 2021 National Library of Norway.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package gowarc

import (
	"errors"
	"github.com/nlnwa/gowarc/internal/diskbuffer"
	"io"
	"io/ioutil"
)

// Block is the interface used to represent the content of a WARC record as specified by the WARC specification:
// https://iipc.github.io/warc-specifications/specifications/warc-format/warc-1.1/#warc-record-content-block
//
// A Block might be cached or non-cached. Calling RawBytes or BlockDigest more than once will fail if the block is not
// cached.
//
// NOTE: Blocks are not required to be thread safe.
type Block interface {
	// RawBytes returns the bytes of the Block
	RawBytes() (io.Reader, error)
	BlockDigest() string
	IsCached() bool
	Cache() error
}

// PayloadBlock is a Block with a well defined payload.
//
// Ref: https://iipc.github.io/warc-specifications/specifications/warc-format/warc-1.1/#warc-record-payload
type PayloadBlock interface {
	Block
	PayloadBytes() (io.Reader, error)
	PayloadDigest() string
}

type genericBlock struct {
	opts              *warcRecordOptions
	rawBytes          io.Reader
	blockDigest       *digest
	filterReader      *digestFilterReader
	blockDigestString string
}

func newGenericBlock(opts *warcRecordOptions, r io.Reader, d *digest) *genericBlock {
	return &genericBlock{opts: opts, rawBytes: r, blockDigest: d}
}

func (block *genericBlock) IsCached() bool {
	_, ok := block.rawBytes.(io.Seeker)
	return ok
}

func (block *genericBlock) Cache() error {
	if block.IsCached() {
		return nil
	}

	r, err := block.RawBytes()
	if err != nil {
		return err
	}

	buf := diskbuffer.New(block.opts.bufferOptions...)
	if _, err := buf.ReadFrom(r); err != nil {
		return err
	}
	if c, ok := block.rawBytes.(io.Closer); ok {
		_ = c.Close()
	}
	block.blockDigestString = block.blockDigest.format()
	block.rawBytes = buf
	return nil
}

func (block *genericBlock) RawBytes() (io.Reader, error) {
	if block.filterReader == nil {
		block.filterReader = newDigestFilterReader(block.rawBytes, block.blockDigest)
		return block.filterReader, nil
	}

	if block.blockDigestString == "" {
		block.BlockDigest()
	}

	if !block.IsCached() {
		return nil, errContentReAccessed
	}

	if _, err := block.rawBytes.(io.Seeker).Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return newDigestFilterReader(block.rawBytes), nil
}

func (block *genericBlock) BlockDigest() string {
	if block.blockDigestString == "" {
		if block.filterReader == nil {
			block.filterReader = newDigestFilterReader(block.rawBytes, block.blockDigest)
		}
		_, _ = io.Copy(ioutil.Discard, block.filterReader)
		block.blockDigestString = block.blockDigest.format()
	}
	return block.blockDigestString
}

var errContentReAccessed = errors.New("gowarc.Block: tried to access content twice")