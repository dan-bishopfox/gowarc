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
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	sphtcrlf = " \t\r\n"  // Space, Tab, Carriage return, Newline
	cr       = '\r'       // Carriage return
	lf       = '\n'       // Newline
	sp       = ' '        // Space
	ht       = '\t'       // Tab
	crlf     = "\r\n"     // Carriage return, Newline
	crlfcrlf = "\r\n\r\n" // Carriage return, Newline, Carriage return, Newline
)

type WarcRecord interface {
	Version() *WarcVersion
	Type() RecordType
	WarcHeader() *WarcFields
	Block() Block
	String() string
	io.Closer
	ToRevisitRecord(ref *RevisitRef) (WarcRecord, error)
	RevisitRef() (*RevisitRef, error)
	Merge(record ...WarcRecord) (WarcRecord, error)
	ValidateDigest(validation *Validation) error
}

type WarcVersion struct {
	id    uint8
	txt   string
	major uint8
	minor uint8
}

func (v *WarcVersion) String() string {
	return "WARC/" + v.txt
}

func (v *WarcVersion) Major() uint8 {
	return v.major
}

func (v *WarcVersion) Minor() uint8 {
	return v.minor
}

var (
	// WARC versions
	V1_0 = &WarcVersion{id: 1, txt: "1.0", major: 1, minor: 0} // WARC 1.0
	V1_1 = &WarcVersion{id: 2, txt: "1.1", major: 1, minor: 1} // WARC 1.1
)

type RecordType uint16

func (rt RecordType) String() string {
	switch rt {
	case 1:
		return "warcinfo"
	case 2:
		return "response"
	case 4:
		return "resource"
	case 8:
		return "request"
	case 16:
		return "metadata"
	case 32:
		return "revisit"
	case 64:
		return "conversion"
	case 128:
		return "continuation"
	default:
		return "unknown"
	}
}

func stringToRecordType(rt string) RecordType {
	switch rt {
	case "warcinfo":
		return 1
	case "response":
		return 2
	case "resource":
		return 4
	case "request":
		return 8
	case "metadata":
		return 16
	case "revisit":
		return 32
	case "conversion":
		return 64
	case "continuation":
		return 128
	default:
		return 0
	}
}

type RevisitRef struct {
	Profile        string
	TargetRecordId string
	TargetUri      string
	TargetDate     string
}

const (
	// WARC record types
	Warcinfo     RecordType = 1
	Response     RecordType = 2
	Resource     RecordType = 4
	Request      RecordType = 8
	Metadata     RecordType = 16
	Revisit      RecordType = 32
	Conversion   RecordType = 64
	Continuation RecordType = 128
)

const (
	// Well known content types
	ApplicationWarcFields = "application/warc-fields"
	ApplicationHttp       = "application/http"
)

const (
	// Well known revisit profiles
	ProfileIdenticalPayloadDigest = "http://netpreserve.org/warc/1.1/revisit/identical-payload-digest"
	ProfileServerNotModified      = "http://netpreserve.org/warc/1.1/revisit/server-not-modified"
)

type warcRecord struct {
	opts       *warcRecordOptions
	version    *WarcVersion
	headers    *WarcFields
	recordType RecordType
	block      Block
	closer     func() error
}

func (wr *warcRecord) Version() *WarcVersion { return wr.version }

func (wr *warcRecord) Type() RecordType { return wr.recordType }

func (wr *warcRecord) WarcHeader() *WarcFields { return wr.headers }

func (wr *warcRecord) Block() Block {
	return wr.block
}

func (wr *warcRecord) String() string {
	return fmt.Sprintf("WARC record: version: %s, type: %s, id: %s", wr.version, wr.Type(), wr.WarcHeader().Get(WarcRecordID))
}

func (wr *warcRecord) Close() error {
	if wr.closer != nil {
		return wr.closer()
	}
	return nil
}

func (wr *warcRecord) ToRevisitRecord(ref *RevisitRef) (WarcRecord, error) {
	h := wr.headers.clone()

	switch ref.Profile {
	case ProfileIdenticalPayloadDigest:
		if !wr.headers.Has(WarcPayloadDigest) {
			return nil, fmt.Errorf("payload digest is required for Identical Payload Digest Profile")
		}
	case ProfileServerNotModified:
	default:
		return nil, fmt.Errorf("unknown revisit profile")
	}

	h.Set(WarcType, Revisit.String())
	h.Set(WarcProfile, ref.Profile)
	if ref.TargetRecordId != "" {
		h.Set(WarcRefersTo, ref.TargetRecordId)
	}
	if ref.TargetUri != "" {
		h.Set(WarcRefersToTargetURI, ref.TargetUri)
	}
	if ref.TargetDate != "" {
		h.Set(WarcRefersToDate, ref.TargetDate)
	}
	h.Set(WarcTruncated, "length")

	block, err := newRevisitBlock(wr.opts, wr.block)
	if err != nil {
		return nil, err
	}
	h.Set(WarcBlockDigest, block.BlockDigest())
	h.Set(WarcPayloadDigest, block.PayloadDigest())
	h.Set(ContentLength, strconv.Itoa(len(block.headerBytes)))

	revisit := &warcRecord{
		opts:       wr.opts,
		version:    wr.version,
		recordType: Revisit,
		headers:    h,
		block:      block,
	}
	return revisit, nil
}

func (wr *warcRecord) RevisitRef() (*RevisitRef, error) {
	if wr.recordType != Revisit {
		return nil, fmt.Errorf("not a revisit record")
	}

	return &RevisitRef{
		Profile:        wr.headers.Get(WarcProfile),
		TargetRecordId: wr.headers.Get(WarcRefersTo),
		TargetUri:      wr.headers.Get(WarcRefersToTargetURI),
		TargetDate:     wr.headers.Get(WarcRefersToDate),
	}, nil
}

func (wr *warcRecord) Merge(record ...WarcRecord) (WarcRecord, error) {
	if wr.headers.Get(WarcSegmentNumber) == "1" {
		return nil, fmt.Errorf("merging of segmentet records is not implemented")
	}
	if wr.recordType != Revisit {
		return nil, fmt.Errorf("merging is only possible for revisit records or segmentet records")
	}
	if len(record) != 1 {
		return nil, fmt.Errorf("a revisit record must be merged with only one referenced record")
	}

	wr.recordType = record[0].Type()
	wr.headers.Set(WarcType, "response")
	wr.headers.Delete(WarcRefersTo)
	wr.headers.Delete(WarcRefersToTargetURI)
	wr.headers.Delete(WarcRefersToDate)
	wr.headers.Delete(WarcProfile)
	if record[0].WarcHeader().Has(WarcTruncated) {
		wr.headers.Set(WarcTruncated, record[0].WarcHeader().Get(WarcTruncated))
	} else {
		wr.headers.Delete(WarcTruncated)
	}

	b, ok := wr.block.(*revisitBlock)
	if !ok {
		return nil, fmt.Errorf("the revisit record's has wrong block type. Creation of record must be done with SkipParseBlock set to false")
	}
	switch v := record[0].Block().(type) {
	case *httpRequestBlock:
		refLen, err := strconv.ParseInt(record[0].WarcHeader().Get(ContentLength), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("could not parse %s", ContentLength)
		}
		size := int64(len(b.headerBytes)) + refLen - int64(len(v.httpHeaderBytes))
		wr.headers.Set(ContentLength, strconv.FormatInt(size, 10))
		v.httpHeaderBytes = b.headerBytes
		wr.block = v
	case *httpResponseBlock:
		refLen, err := strconv.ParseInt(record[0].WarcHeader().Get(ContentLength), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("could not parse %s", ContentLength)
		}
		size := int64(len(b.headerBytes)) + refLen - int64(len(v.httpHeaderBytes))
		wr.headers.Set(ContentLength, strconv.FormatInt(size, 10))
		v.httpHeaderBytes = b.headerBytes
		wr.block = v
	default:
		return nil, fmt.Errorf("merging of revisits is only implemented for http requests and responses")
	}
	if record[0].Block().IsCached() {
		wr.headers.Set(WarcBlockDigest, record[0].Block().BlockDigest())
	} else {
		wr.headers.Delete(WarcBlockDigest)
	}

	return wr, nil
}

func (wr *warcRecord) parseBlock(reader io.Reader, validation *Validation) (err error) {
	blockDigest, err := newDigestFromField(wr, WarcBlockDigest)
	if err != nil {
		return err
	}
	payloadDigest, err := newDigestFromField(wr, WarcPayloadDigest)
	if err != nil {
		return err
	}

	if !wr.opts.skipParseBlock {
		contentType := strings.ToLower(wr.headers.Get(ContentType))
		if wr.recordType&(Response|Resource|Request|Conversion|Continuation) != 0 {
			if strings.HasPrefix(contentType, ApplicationHttp) {
				httpBlock, err := newHttpBlock(wr.opts, reader, blockDigest, payloadDigest)
				if err != nil {
					return err
				}
				wr.block = httpBlock
				return nil
			}
		}
		if wr.recordType == Revisit {
			revisitBlock, err := parseRevisitBlock(wr.opts, reader, blockDigest, wr.headers.Get(WarcPayloadDigest))
			if err != nil {
				return err
			}
			wr.block = revisitBlock
			return nil
		}
		if strings.HasPrefix(contentType, ApplicationWarcFields) {
			warcFieldsBlock, err := newWarcFieldsBlock(reader, blockDigest, validation, wr.opts)
			if err != nil {
				return err
			}
			wr.block = warcFieldsBlock
			return nil
		}
	}

	wr.block = newGenericBlock(wr.opts, reader, blockDigest)
	return
}

// ValidateDigest validates block and payload digests if present.
// If option FixDigest is set, an invalid or missing digest will be corrected in the header.
// If the record is not cached, it might not be possible to read any content from this record after validation.
func (wr *warcRecord) ValidateDigest(validation *Validation) error {
	wr.Block().BlockDigest()

	var blockDigest, payloadDigest *digest
	switch v := wr.Block().(type) {
	case *genericBlock:
		blockDigest = v.blockDigest
	case *httpRequestBlock:
		blockDigest = v.blockDigest
		payloadDigest = v.payloadDigest
	case *httpResponseBlock:
		blockDigest = v.blockDigest
		payloadDigest = v.payloadDigest
	case *revisitBlock:
		blockDigest = v.blockDigest
	case *warcFieldsBlock:
		blockDigest = v.blockDigest
	}

	if blockDigest != nil {
		if blockDigest.hash == "" {
			// Missing digest header is allowed, so skip validation. But if fixDigest option is set, a header will be added.
			if wr.opts.fixDigest {
				wr.WarcHeader().Set(WarcBlockDigest, blockDigest.format())
				return nil
			}
		} else {
			if err := blockDigest.validate(); err != nil {
				switch wr.opts.errSpec {
				case ErrIgnore:
				case ErrWarn:
					validation.addError(err)
					if wr.opts.fixDigest {
						wr.WarcHeader().Set(WarcBlockDigest, blockDigest.format())
					}
				case ErrFail:
					return fmt.Errorf("wrong block digest " + err.Error())
				}
			}
		}
	}

	if wr.Type() == Revisit || wr.WarcHeader().Has(WarcSegmentNumber) {
		// Can't check payload digest for revisit records or segmented records since the payload digest is a digest of
		// the original record
		return nil
	}

	if payloadDigest != nil {
		if payloadDigest.hash == "" {
			// Missing digest header is allowed, so skip validation. But if fixDigest option is set, a header will be added.
			if wr.opts.fixDigest {
				wr.WarcHeader().Set(WarcPayloadDigest, payloadDigest.format())
				return nil
			}
		} else {
			if err := payloadDigest.validate(); err != nil {
				switch wr.opts.errSpec {
				case ErrIgnore:
				case ErrWarn:
					validation.addError(err)
					if wr.opts.fixDigest {
						wr.WarcHeader().Set(WarcPayloadDigest, payloadDigest.format())
					}
				case ErrFail:
					return fmt.Errorf("wrong payload digest " + err.Error())
				}
			}
		}
	}
	return nil
}
