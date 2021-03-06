// Copyright 2016 Giulio Iotti. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package store

import "time"

type BuildResult struct {
	Start  time.Time
	End    time.Time
	Act    BuildAct
	Stdout []byte
	Stderr []byte
	Retval int
	Ticket int64
	Cmd    string
	Stage  string
	Branch string
	SHA1   string
}

type Store interface {
	Add(br *BuildResult) error
	Get(stage string) ([]*BuildResult, error)
	Delete(stage string) error
	Clean(until time.Time) error
}

type BuildAct int

const (
	BuildActCreate BuildAct = iota
	BuildActUpdate
	BuildActChange
	BuildActDestroy
)

func (a BuildAct) String() string {
	switch a {
	case BuildActCreate:
		return "create"
	case BuildActUpdate:
		return "update"
	case BuildActChange:
		return "change"
	case BuildActDestroy:
		return "destroy"
	}
	return "unknown"
}
