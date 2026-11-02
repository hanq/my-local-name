package main

import (
	"fmt"
	"runtime"
)

// Version is shown in the UI and the tray tooltip.
const Version = "0.1.0"

// Event names shared with the frontend.
const (
	EventStateUpdate = "state:update"
	EventLogLine     = "log:line"
)

// InterfaceOption is one selectable network interface.
type InterfaceOption struct {
	Index     int      `json:"index"`
	Name      string   `json:"name"`
	Addresses []string `json:"addresses"`
	Selected  bool     `json:"selected"`
}

// LogLine mirrors applog.Entry for the frontend.
type LogLine struct {
	ID      int64  `json:"id"`
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

// AppState is the single source of truth for the UI.
type AppState struct {
	Status             string            `json:"status"`
	Prefix             string            `json:"prefix"`
	Suffix             string            `json:"suffix"`
	FQDN               string            `json:"fqdn"`
	Addresses          []string          `json:"addresses"`
	Interfaces         []InterfaceOption `json:"interfaces"`
	SelectedInterfaces []int             `json:"selectedInterfaces"`
	Error              string            `json:"error"`
	Suggestion         string            `json:"suggestion"`
	Since              int64             `json:"since"`
	StartedAt          int64             `json:"startedAt"`
	LastAnnounceAt     int64             `json:"lastAnnounceAt"`
	QueriesAnswered    int64             `json:"queriesAnswered"`
	AnnouncementsSent  int64             `json:"announcementsSent"`
	ConflictsDetected  int64             `json:"conflictsDetected"`
	AutostartEnabled   bool              `json:"autostartEnabled"`
	AutostartSupported bool              `json:"autostartSupported"`
	AutostartStrategy  string            `json:"autostartStrategy"`
	AutoBroadcast      bool              `json:"autoBroadcast"`
	StartMinimized     bool              `json:"startMinimized"`
	Port               int               `json:"port"`
	StandardSuffix     bool              `json:"standardSuffix"`
	TestCommand        string            `json:"testCommand"`
	Version            string            `json:"version"`
	Platform           string            `json:"platform"`
}

// Diagnostics is returned by GetDiagnostics for the settings drawer.
type Diagnostics struct {
	FQDN           string   `json:"fqdn"`
	Port           string   `json:"port"`
	Group          string   `json:"group"`
	ConfigPath     string   `json:"configPath"`
	LogPath        string   `json:"logPath"`
	Interfaces     []string `json:"interfaces"`
	Commands       []string `json:"commands"`
	Notes          []string `json:"notes"`
	SuffixStandard bool     `json:"suffixStandard"`
}

func resolverCommands(fqdn string) []string {
	switch runtime.GOOS {
	case "windows":
		return []string{
			"Resolve-DnsName " + fqdn + " -Type A",
			"ping " + fqdn,
		}
	case "darwin":
		return []string{
			"dns-sd -G v4 " + fqdn,
			"ping " + fqdn,
		}
	default:
		return []string{
			"avahi-resolve -n " + fqdn,
			"resolvectl query " + fqdn,
		}
	}
}

func isActiveState(status string) bool {
	return status == "probing" || status == "announcing" || status == "broadcasting"
}

func portText(port int) string {
	return fmt.Sprintf("%d/UDP", port)
}

func platformName() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows"
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	default:
		return runtime.GOOS
	}
}
