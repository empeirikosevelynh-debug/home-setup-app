package domain

import "io/fs"

type Options struct {
	RecoveryDate                                                  string
	Apps, Plugins, Languages                                      []string
	Workspaces                                                    []Workspace
	ConfigureGit, AdoptChezmoi, CaptureInventory, PrepareRecovery bool
	FileChoices                                                   map[string]FileDecision
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
}
type Check struct{ Kind, Target, Expected string }
type Step struct {
	ID, Label, Kind string
	DependsOn       []string
	BeforeFiles     []FileState
	Package         *Package    `json:",omitempty"`
	Command         *Command    `json:",omitempty"`
	File            *FileChange `json:",omitempty"`
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
