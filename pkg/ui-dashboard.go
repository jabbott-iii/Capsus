/*
Copyright 2026 Joseph Anthony Abbott III

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pkg

import (
	tea "github.com/charmbracelet/bubbletea"
)

//--------------------------------------------------messages-------------------------------------------------------------------------------------//

type keysLoadedMsg struct{ keys []KeyModel }
type errMsg struct{ err error }

// actionResultMsg carries the outcome of running a background file/key
// operation triggered from a form screen. See logic-tui.go.
type actionResultMsg struct {
	message string
	err     error
	reload  bool // true if the stored key list should be reloaded
}

type dashboardOptions struct {
	vimEnabled bool
}

//--------------------------------------------------model----------------------------------------------------------------------------------------//

type dashboardScreen int

const (
	screenMain dashboardScreen = iota
	screenKeys
	screenForm
)

type formInputMode int

const (
	formModeInsert formInputMode = iota
	formModeNormal
)

// actionKind identifies which file/key operation a form screen submits.
type actionKind int

const (
	actionNone actionKind = iota
	actionEncrypt
	actionDecrypt
	actionCompress
	actionDecompress
	actionKeysGenerate
	actionKeysExport
	actionKeysImport
	actionKeysDelete
)

// formField is a single editable text field rendered on the form screen.
type formField struct {
	label    string
	value    []rune
	password bool
}

// DashboardModel is the root TUI model.
type DashboardModel struct {
	db      *Database
	screen  dashboardScreen
	cursor  int
	keys    []KeyModel
	status  string
	isError bool
	busy    bool
	width   int
	height  int

	// form state, used when screen == screenForm
	action     actionKind
	fields     []formField
	fieldIdx   int
	formOrigin dashboardScreen
	formMode   formInputMode
	vimEnabled bool

	// runner runs form actions so they can be cancelled; quitting is set when the
	// user quit while an action ran, and the program ends once it reports back.
	runner   *actionRunner
	quitting bool
}

var mainMenuItems = []string{
	"Encrypt a file or directory",
	"Decrypt a file or directory",
	"Compress a file or directory",
	"Decompress a file or archive",
	"Manage keys",
}

var keysMenuItems = []string{
	"Generate a new key",
	"Export a key",
	"Import a key",
	"Delete a key",
}

// NewDashboardModel creates the initial dashboard model.
func NewDashboardModel(db *Database) DashboardModel {
	return NewDashboardModelWithOptions(db, dashboardOptions{})
}

// NewDashboardModelWithOptions creates the initial dashboard model with the
// provided dashboard options.
func NewDashboardModelWithOptions(db *Database, opts dashboardOptions) DashboardModel {
	return DashboardModel{
		db:         db,
		formMode:   formModeInsert,
		vimEnabled: opts.vimEnabled,
		runner:     &actionRunner{},
	}
}

//--------------------------------------------------bubbletea interface--------------------------------------------------------------------------//

func (m DashboardModel) Init() tea.Cmd {
	return loadKeysCmd(m.db)
}

//--------------------------------------------------commands-------------------------------------------------------------------------------------//

func loadKeysCmd(db *Database) tea.Cmd {
	return func() tea.Msg {
		keys, err := db.ListKeys()
		if err != nil {
			return errMsg{err}
		}
		return keysLoadedMsg{keys}
	}
}
