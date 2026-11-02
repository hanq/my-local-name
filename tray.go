package main

import (
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// trayUI keeps references to the menu items whose labels change with state.
type trayUI struct {
	app           *application.App
	svc           *AppService
	tray          *application.SystemTray
	statusItem    *application.MenuItem
	toggleItem    *application.MenuItem
	copyItem      *application.MenuItem
	autostartItem *application.MenuItem
}

func setupTray(app *application.App, svc *AppService) *trayUI {
	t := &trayUI{app: app, svc: svc}
	tray := app.SystemTray.New()
	t.tray = tray

	menu := app.NewMenu()
	menu.AddSectionHeader("MY LOCAL NAME")
	t.statusItem = menu.Add("待机")
	t.statusItem.SetEnabled(false)
	menu.AddSeparator()
	menu.Add("显示主窗口").OnClick(func(_ *application.Context) {
		svc.ShowWindow()
	})
	t.toggleItem = menu.Add("开始广播").OnClick(func(_ *application.Context) {
		if _, err := svc.ToggleBroadcast(); err != nil {
			svc.ShowWindow()
		}
	})
	t.copyItem = menu.Add("复制地址").OnClick(func(_ *application.Context) {
		st := svc.GetState()
		if st.FQDN != "" {
			app.Clipboard.SetText(st.FQDN)
		}
	})
	menu.AddSeparator()
	t.autostartItem = menu.AddCheckbox("开机自启", false).OnClick(func(ctx *application.Context) {
		want := ctx.ClickedMenuItem().Checked()
		if _, err := svc.SetAutostart(want); err != nil {
			ctx.ClickedMenuItem().SetChecked(!want)
			app.Dialog.Error().SetTitle("开机自启").SetMessage(err.Error()).Show()
		}
	})
	menu.AddSeparator()
	menu.Add("退出").OnClick(func(_ *application.Context) {
		svc.Quit()
	})

	tray.SetMenu(menu)
	if runtime.GOOS == "darwin" {
		tray.SetTemplateIcon(trayTemplateIcon)
	} else {
		tray.SetIcon(trayIcon)
	}
	tray.SetTooltip("MY LOCAL NAME · 未广播")
	tray.OnClick(func() {
		svc.ShowWindow()
	})

	t.refresh(svc.GetState())
	return t
}

func (t *trayUI) refresh(st AppState) {
	if t == nil || t.tray == nil {
		return
	}

	tooltip := "MY LOCAL NAME · 未广播"
	status := "待机"
	switch st.Status {
	case "probing":
		status = "正在探测名字冲突"
		tooltip = "MY LOCAL NAME · 探测 " + st.FQDN
	case "announcing":
		status = "正在宣告 " + st.FQDN
		tooltip = "MY LOCAL NAME · 宣告 " + st.FQDN
	case "broadcasting":
		status = "广播中 · " + st.FQDN
		tooltip = "MY LOCAL NAME · " + st.FQDN
	case "conflict":
		status = "名称冲突 · " + st.FQDN
		tooltip = "MY LOCAL NAME · 名称冲突"
	case "error":
		status = "启动失败"
		tooltip = "MY LOCAL NAME · 启动失败"
	}

	t.statusItem.SetLabel(status)

	switch st.Status {
	case "probing", "announcing":
		t.toggleItem.SetLabel("正在启动…")
		t.toggleItem.SetEnabled(false)
	case "broadcasting":
		t.toggleItem.SetLabel("停止广播")
		t.toggleItem.SetEnabled(true)
	default:
		t.toggleItem.SetLabel("开始广播")
		t.toggleItem.SetEnabled(true)
	}

	t.copyItem.SetEnabled(st.FQDN != "")
	t.autostartItem.SetChecked(st.AutostartEnabled)
	t.autostartItem.SetEnabled(st.AutostartSupported)
	t.tray.SetTooltip(tooltip)
}
