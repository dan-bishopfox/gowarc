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

import "github.com/nlnwa/gowarc/internal/diskbuffer"

type warcRecordOptions struct {
	warcVersion             *WarcVersion
	errSyntax               errorPolicy
	errSpec                 errorPolicy
	errUnknowRecordType     errorPolicy
	skipParseBlock          bool
	addMissingRecordId      bool
	addMissingContentLength bool
	addMissingDigest        bool
	fixContentLength        bool
	fixDigest               bool
	defaultDigestAlgorithm  string
	bufferOptions           []diskbuffer.Option
}

// The errorPolicy constants describe how to handle WARC record errors.
type errorPolicy int8

const (
	ErrIgnore errorPolicy = 0 // Ignore the given error.
	ErrWarn   errorPolicy = 1 // Ignore given error, but submit a warning.
	ErrFail   errorPolicy = 2 // Fail on given error.
)

// WarcRecordOption configures validation, marshaling and unmarshaling of WARC records.
type WarcRecordOption interface {
	apply(*warcRecordOptions)
}

// funcWarcRecordOption wraps a function that modifies warcRecordOptions into an
// implementation of the WarcRecordOption interface.
type funcWarcRecordOption struct {
	f func(*warcRecordOptions)
}

func (fo *funcWarcRecordOption) apply(po *warcRecordOptions) {
	fo.f(po)
}

func newFuncWarcRecordOption(f func(*warcRecordOptions)) *funcWarcRecordOption {
	return &funcWarcRecordOption{
		f: f,
	}
}

func defaultWarcRecordOptions() warcRecordOptions {
	return warcRecordOptions{
		warcVersion:             V1_1,
		errSyntax:               ErrWarn,
		errSpec:                 ErrWarn,
		errUnknowRecordType:     ErrWarn,
		skipParseBlock:          false,
		addMissingRecordId:      true,
		addMissingContentLength: true,
		addMissingDigest:        true,
		defaultDigestAlgorithm:  "sha1",
		fixContentLength:        true,
		fixDigest:               true,
	}
}

// New creates a new configuration with the supplied warcRecordOptions.
func newOptions(opts ...WarcRecordOption) *warcRecordOptions {
	o := defaultWarcRecordOptions()
	for _, opt := range opts {
		opt.apply(&o)
	}
	return &o
}

// WithVersion sets the WARC version to use for new records
// defaults to WARC/1.1
func WithVersion(version *WarcVersion) WarcRecordOption {
	return newFuncWarcRecordOption(func(o *warcRecordOptions) {
		o.warcVersion = version
	})
}

// WithSyntaxErrorPolicy sets the policy for handling syntax errors in WARC records
// defaults to ErrWarn
func WithSyntaxErrorPolicy(policy errorPolicy) WarcRecordOption {
	return newFuncWarcRecordOption(func(o *warcRecordOptions) {
		o.errSyntax = policy
	})
}

// WithSpecViolationPolicy sets the policy for handling violations of the WARC specification in WARC records
// defaults to ErrWarn
func WithSpecViolationPolicy(policy errorPolicy) WarcRecordOption {
	return newFuncWarcRecordOption(func(o *warcRecordOptions) {
		o.errSpec = policy
	})
}

// WithUnknownRecordTypePolicy sets the policy for handling unknown record types
// defaults to ErrWarn
func WithUnknownRecordTypePolicy(policy errorPolicy) WarcRecordOption {
	return newFuncWarcRecordOption(func(o *warcRecordOptions) {
		o.errUnknowRecordType = policy
	})
}

// WithAddMissingRecordId sets if missing WARC-Record-ID header should be generated.
// defaults to true
func WithAddMissingRecordId(addMissingRecordId bool) WarcRecordOption {
	return newFuncWarcRecordOption(func(o *warcRecordOptions) {
		o.addMissingRecordId = addMissingRecordId
	})
}

// WithAddMissingContentLength sets if missing Content-Length header should be calculated.
// defaults to true
func WithAddMissingContentLength(addMissingContentLength bool) WarcRecordOption {
	return newFuncWarcRecordOption(func(o *warcRecordOptions) {
		o.addMissingContentLength = addMissingContentLength
	})
}

// WithAddMissingDigest sets if missing Block digest and eventually Payload digest header fields should be calculated.
// Only fields which can be generated automaticly are added. That includes WarcRecordID, ContentLength, BlockDigest and PayloadDigest.
// defaults to true
func WithAddMissingDigest(addMissingDigest bool) WarcRecordOption {
	return newFuncWarcRecordOption(func(o *warcRecordOptions) {
		o.addMissingDigest = addMissingDigest
	})
}

// WithDefaultDigestAlgorithm sets which algorihm to use for digest generation.
// Valid values: 'md5', 'sha1', 'sha256' and 'sha512'.
// defaults to sha1
func WithDefaultDigestAlgorithm(defaultDigestAlgorithm string) WarcRecordOption {
	return newFuncWarcRecordOption(func(o *warcRecordOptions) {
		o.defaultDigestAlgorithm = defaultDigestAlgorithm
	})
}

// WithFixContentLength sets if a ContentLength header with value which do not match the actual content length should be set to the real value.
// This will not have any impact if SpecViolationPolicy is ErrIgnore
// defaults to true
func WithFixContentLength(fixContentLength bool) WarcRecordOption {
	return newFuncWarcRecordOption(func(o *warcRecordOptions) {
		o.fixContentLength = fixContentLength
	})
}

// WithFixDigest sets if a BlockDigest header or a PayloadDigest header with a value which do not match the actual content should be recalculated.
// This will not have any impact if SpecViolationPolicy is ErrIgnore
// defaults to true
func WithFixDigest(fixDigest bool) WarcRecordOption {
	return newFuncWarcRecordOption(func(o *warcRecordOptions) {
		o.fixDigest = fixDigest
	})
}

// WithSkipParseBlock sets parser to skip detecting known block types.
// This implies that no payload digest can be computed.
func WithSkipParseBlock() WarcRecordOption {
	return newFuncWarcRecordOption(func(o *warcRecordOptions) {
		o.skipParseBlock = true
	})
}

// WithNoValidation sets the parser to do as little validation as possible.
//
// This option is for parsing as fast as possible and beeing as lenient as possible.
// Settings implied by this option are:
//   SyntaxErrorPolicy = ErrIgnore
//   SpecViolationPolicy = ErrIgnore
//   UnknownRecordPolicy = ErrIgnore
//   SkipParseBlock = true
func WithNoValidation() WarcRecordOption {
	return newFuncWarcRecordOption(func(o *warcRecordOptions) {
		o.errSyntax = ErrIgnore
		o.errSpec = ErrIgnore
		o.errUnknowRecordType = ErrIgnore
		o.skipParseBlock = true
	})
}

// WithStrictValidation sets the parser to fail on first error or violation of WARC specification.
//
// Settings implied by this option are:
//   SyntaxErrorPolicy = ErrFail
//   SpecViolationPolicy = ErrFail
//   UnknownRecordPolicy = ErrFail
//   SkipParseBlock = false
func WithStrictValidation() WarcRecordOption {
	return newFuncWarcRecordOption(func(o *warcRecordOptions) {
		o.errSyntax = ErrFail
		o.errSpec = ErrFail
		o.errUnknowRecordType = ErrFail
		o.skipParseBlock = false
	})
}

// WithBufferTmpDir sets the directory to use for temporary files.
// If not set or dir is the empty string then the default directory for temporary files is used (see os.TempDir).
func WithBufferTmpDir(dir string) WarcRecordOption {
	return newFuncWarcRecordOption(func(o *warcRecordOptions) {
		o.bufferOptions = append(o.bufferOptions, diskbuffer.WithTmpDir(dir))
	})
}

// WithBufferMaxMemBytes sets the maximum amount of memory a buffer is allowed to use before overflowing to disk.
// defaults to 1 MiB
func WithBufferMaxMemBytes(size int64) WarcRecordOption {
	return newFuncWarcRecordOption(func(o *warcRecordOptions) {
		o.bufferOptions = append(o.bufferOptions, diskbuffer.WithMaxMemBytes(size))
	})
}