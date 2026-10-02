package domain

import "io/fs"

type Options struct {
	RecoveryDate                                                  string
	Apps, Plugins, Languages                                      []string
	Workspaces                                                    []Workspace
	ConfigureGit, AdoptChezmoi, CaptureInventory, PrepareRecovery bool
	FileChoices                                                   map[string]FileDecision
	// Bringing over a previous Mac. Empty values are left out of the JSON,
	// so plans that do not use them keep their IDs.
	PreviousBrewfile string   `json:",omitempty"`
	PreviousPackages []string `json:",omitempty"`
	// ImportFrom is a previous home folder; ImportFolders are names directly
	// inside it, or "." for the files at its top.
	ImportFrom    string   `json:",omitempty"`
	ImportFolders []string `json:",omitempty"`
	// DotfilesRepo is a chezmoi dotfiles repository: a GitHub user,
	// user/repo or a Git URL.
	DotfilesRepo string `json:",omitempty"`
}
type Workspace struct {
	Language, Path, Module, EntryPoint string
	Create                             bool
}
type WorkspaceState struct{ Exists, Directory, Empty, Symlink bool }
type Host struct {
	Workspaces                                                                        map[string]WorkspaceState
	FishInstalledPlugins                                                              []string
	FishLegacy                                                                        bool
	OS, Arch, Version, Home, BrewPath, BrewPrefix, LazyGitDir, ChezmoiDir, ConfigHome string
	Rosetta, ChezmoiDirty, FishPluginConflict                                         bool
	Packages                                                                          map[string]InstalledPackage
	Apps                                                                              []AppBundle
	Files                                                                             map[string]FileState
	Tools                                                                             map[string]string
	// Windows only. Empty values are left out of the JSON, so macOS
	// inspection data, and the plan IDs derived from it, are unchanged.
	ChocoPath, ChocoVersion, AppData string `json:",omitempty"`
	Elevated                         bool   `json:",omitempty"`
	// Read only when a previous Mac's app list is chosen.
	Taps, PreviousEntries, PreviousOther []string `json:",omitempty"`
	// Read only when a previous home folder is chosen. ConflictDates names
	// the dated folders earlier imports saved conflicts in. Free space changes
	// all the time, so it stays out of the inspection fingerprint.
	Import         []ImportScan `json:",omitempty"`
	ImportProblems []string     `json:",omitempty"`
	ConflictDates  []string     `json:",omitempty"`
	FreeBytes      int64        `json:"-"`
	// Read only when a dotfiles repository is chosen. DotfilesState is
	// missing (not cloned yet), waiting (cloned, chezmoi not installed),
	// cloned, or other (chezmoi's source holds other dotfiles, from
	// DotfilesOrigin). Dotfiles are written as they are; DotfilesManual
	// need chezmoi itself.
	DotfilesState   string    `json:",omitempty"`
	DotfilesOrigin  string    `json:",omitempty"`
	Dotfiles        []Dotfile `json:",omitempty"`
	DotfilesManual  []string  `json:",omitempty"`
	DotfilesScripts bool      `json:",omitempty"`
	DotfilesProblem string    `json:",omitempty"`
}

// Dotfile is a file a dotfiles repository restores as it is. Create files
// are only written where nothing is.
type Dotfile struct {
	Target   string
	Source   FileSource
	Mode     fs.FileMode
	Create   bool   `json:",omitempty"`
	Contents []byte `json:"-"`
}

// FileSource is where a restored file's contents are read when it is
// written, so they never enter session records.
type FileSource struct{ Root, Path, SHA256 string }

// ImportJob copies one folder of a previous home folder into this home.
// Existing files are never replaced: a different version is saved under
// Conflicts instead.
type ImportJob struct {
	Source, Destination, Conflicts string
	// TopFilesOnly copies only the files directly inside Source.
	TopFilesOnly bool `json:",omitempty"`
	// Exclude holds destination paths that another step restores.
	Exclude []string `json:",omitempty"`
	// Saved holds earlier conflicts folders: a copy there counts as saved.
	Saved []string `json:",omitempty"`
}

// ImportScan counts what importing a folder would do, or did.
// Aside counts files whose version here differs but whose source copy is
// already saved in a conflicts folder.
type ImportScan struct {
	Folder                                 string
	Copy, Same, Differ, CloudOnly, Special int
	Aside                                  int `json:",omitempty"`
	CopyBytes, DifferBytes                 int64
	Digest                                 string `json:",omitempty"`
}
type InstalledPackage struct {
	Version   string
	OnRequest *bool
}
type AppBundle struct {
	Name, Path               string
	Registered, StoreReceipt bool
}
type FileState struct {
	Path, SHA256    string
	Mode            fs.FileMode
	Exists, Symlink bool
	Contents        []byte `json:"-"`
}
type Package struct{ Kind, Token, MinVersion string }
type Command struct {
	Path, Dir   string
	Args, Env   []string
	UnsetEnv    []string
	Interactive bool
	Stream      bool
}
type FileDecision string

const (
	Preserve FileDecision = "preserve"
	Create   FileDecision = "create"
	Replace  FileDecision = "replace"
)

type FileChange struct {
	Path, BeforeSHA256 string
	BeforeExists       bool
	Mode               fs.FileMode
	Desired            []byte
	Decision           FileDecision
	// Source replaces Desired for a restored file.
	Source *FileSource `json:",omitempty"`
}
type Check struct{ Kind, Target, Expected string }
type Step struct {
	ID, Label, Kind string
	DependsOn       []string
	BeforeFiles     []FileState
	Package         *Package    `json:",omitempty"`
	Command         *Command    `json:",omitempty"`
	File            *FileChange `json:",omitempty"`
	Import          *ImportJob  `json:",omitempty"`
	Check           Check
}
type Plan struct {
	SchemaVersion int
	ID            string
	InspectionID  string
	Supported     bool
	Accepted      bool `json:"-"`
	Problems      []string
	Options       Options
	Steps         []Step
	ManualTasks   []ManualTask
	// Later says what a second review plans once this plan is applied.
	Later string `json:",omitempty"`
}
type ManualTask struct {
	ID, Title, Instructions, URL string
	Required                     bool
}
type Event struct{ StepID, Status, Text string }
type Report struct {
	PlanID, Status, SessionPath string
	Steps                       []StepResult
	ManualTasks                 []ManualTask
}
type StepResult struct{ ID, Status, Message, BackupPath string }
type Session struct {
	Status        string
	SchemaVersion int
	Plan          Plan
	Results       []StepResult
}
