package main

import (
	"embed"
	"log"
	"log/slog"
	"os"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed assets/tray.png
var trayIcon []byte

//go:embed assets/tray_template.png
var trayTemplateIcon []byte

//go:embed assets/appicon.png
var appIcon []byte

func init() {
	application.RegisterEvent[AppState](EventStateUpdate)
	application.RegisterEvent[LogLine](EventLogLine)
}

func main() {
	autostart := hasArg("--autostart")
	forceShow := hasArg("--show")

	svc := NewAppService()
	svc.autostartLaunch = autostart
	cfg := svc.cfgSnapshot()
	startHidden := !forceShow && (autostart || cfg.StartMinimized)

	app := application.New(application.Options{
		Name:        "MY LOCAL NAME",
		Description: "Advertise a custom .local hostname on the local network over mDNS",
		Icon:        appIcon,
		LogLevel:    slog.LevelInfo,
		Services: []application.Service{
			application.NewService(svc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "cn.hanq.mylocalname",
			OnSecondInstanceLaunch: func(_ application.SecondInstanceData) {
				svc.ShowWindow()
			},
		},
		Mac: application.MacOptions{
			ActivationPolicy: application.ActivationPolicyAccessory,
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
		},
		OnShutdown: svc.shutdown,
	})
	svc.app = app

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            "MY LOCAL NAME",
		Width:            800,
		Height:           600,
		MinWidth:         340,
		MinHeight:        540,
		Frameless:        true,
		Hidden:           startHidden,
		URL:              "/",
		BackgroundColour: application.NewRGB(20, 10, 17),
		Mac: application.MacWindow{
			TitleBar:                application.MacTitleBarHidden,
			InvisibleTitleBarHeight: 44,
			Backdrop:                application.MacBackdropNormal,
		},
		Windows: application.WindowsWindow{
			Theme: application.Dark,
		},
		KeyBindings: map[string]func(application.Window){
			"F9": func(_ application.Window) {
				_, _ = svc.ToggleBroadcast()
			},
		},
	})
	svc.window = window

	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		window.Hide()
		e.Cancel()
	})

	svc.tray = setupTray(app, svc)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func hasArg(want string) bool {
	for _, arg := range os.Args[1:] {
		if strings.EqualFold(strings.TrimSpace(arg), want) {
			return true
		}
	}
	return false
}
