//go:build experimental_wayland

package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/frontend"
	"github.com/rajveermalviya/go-wayland/wayland/client"
	"github.com/rajveermalviya/go-wayland/wayland/stable/xdg-shell"
	"golang.org/x/sys/unix"
)

type appState struct {
	display    *client.Display
	registry   *client.Registry
	compositor *client.Compositor
	shm        *client.Shm
	wmBase     *xdg_shell.WmBase
	surface    *client.Surface
	xdgSurface *xdg_shell.Surface
	toplevel   *xdg_shell.Toplevel
	logger     *slog.Logger
	width      int32
	height     int32
}

func main() {
	socketAddr := flag.String("socket", "tcp://127.0.0.1:4242", "Alloy core IPC address")
	insecure := flag.Bool("insecure", true, "Disable mTLS for local testing")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	logger.Info("Starting Pure Go Wayland Native GUI", "addr", *socketAddr)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 1. Initialize shared frontend client (IPC part)
	ipcClient, err := frontend.NewClient("alloy-gui-wayland", *socketAddr, *insecure)
	if err != nil {
		logger.Error("failed to create frontend client", "error", err)
		os.Exit(1)
	}
	defer ipcClient.Close()

	// 2. Pure Go Wayland connection
	app := &appState{
		logger: logger,
		width:  800,
		height: 600,
	}

	display, err := client.Connect("")
	if err != nil {
		logger.Error("failed to connect to Wayland compositor", "error", err)
		os.Exit(1)
	}
	app.display = display
	defer display.Context().Close()

	registry, err := display.GetRegistry()
	if err != nil {
		logger.Error("failed to get registry", "error", err)
		os.Exit(1)
	}
	app.registry = registry

	registry.SetGlobalHandler(app.handleGlobal)
	// Sync twice to ensure all globals are received and bound
	for i := 0; i < 2; i++ {
		callback, _ := display.Sync()
		callback.SetDoneHandler(func(client.CallbackDoneEvent) {})
		if err := display.Context().Dispatch(); err != nil {
			logger.Error("Dispatch error", "error", err)
			os.Exit(1)
		}
	}

	if app.compositor == nil || app.shm == nil || app.wmBase == nil {
		logger.Error("required wayland globals not found")
		os.Exit(1)
	}

	// 3. Setup Surface and XDG Objects
	app.surface, _ = app.compositor.CreateSurface()
	app.xdgSurface, _ = app.wmBase.GetXdgSurface(app.surface)
	app.xdgSurface.SetConfigureHandler(app.handleXdgSurfaceConfigure)

	app.toplevel, _ = app.xdgSurface.GetToplevel()
	app.toplevel.SetTitle("Alloy Pure Go Wayland")
	app.toplevel.SetCloseHandler(func(xdg_shell.ToplevelCloseEvent) { cancel() })

	app.surface.Commit()
	callback, _ := display.Sync()
	callback.SetDoneHandler(func(client.CallbackDoneEvent) {})
	display.Context().Dispatch()

	// 4. Initial Draw
	app.paint(color.RGBA{20, 20, 25, 255}) // Dark background

	// Dispatch Loop
	go func() {
		for {
			if err := display.Context().Dispatch(); err != nil {
				logger.Error("Wayland dispatch error", "error", err)
				return
			}
		}
	}()

	// Alloy Core Integration
	ipcClient.OnMessage(func(msg api.Message) {
		logger.Info("Kernel Event received", "method", msg.Method, "id", msg.ID)
		// Flash blue on message
		app.paint(color.RGBA{40, 60, 120, 255})
		time.Sleep(100 * time.Millisecond)
		app.paint(color.RGBA{20, 20, 25, 255})
	})

	<-ctx.Done()
	logger.Info("GUI shutting down")
}

func (app *appState) handleGlobal(ev client.RegistryGlobalEvent) {
	app.logger.Debug("Wayland global found", "interface", ev.Interface, "version", ev.Version, "name", ev.Name)
	switch ev.Interface {
	case "wl_compositor":
		app.compositor = client.NewCompositor(app.display.Context())
		app.registry.Bind(ev.Name, ev.Interface, ev.Version, app.compositor)
	case "wl_shm":
		app.shm = client.NewShm(app.display.Context())
		app.registry.Bind(ev.Name, ev.Interface, ev.Version, app.shm)
	case "xdg_wm_base":
		app.wmBase = xdg_shell.NewWmBase(app.display.Context())
		app.registry.Bind(ev.Name, ev.Interface, ev.Version, app.wmBase)
	}
}

func (app *appState) handleXdgSurfaceConfigure(ev xdg_shell.SurfaceConfigureEvent) {
	app.xdgSurface.AckConfigure(ev.Serial)
}

func (app *appState) paint(bgColor color.RGBA) {
	img := image.NewRGBA(image.Rect(0, 0, int(app.width), int(app.height)))
	draw.Draw(img, img.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)

	// Draw some text-like box
	draw.Draw(img, image.Rect(50, 50, 200, 100), &image.Uniform{color.RGBA{200, 200, 200, 255}}, image.Point{}, draw.Over)

	size := app.width * app.height * 4
	file, err := createShmFile(int(size))
	if err != nil {
		app.logger.Error("failed to create shm file", "error", err)
		return
	}
	defer file.Close()

	data, err := unix.Mmap(int(file.Fd()), 0, int(size), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		app.logger.Error("mmap failed", "error", err)
		return
	}
	defer unix.Munmap(data)

	copy(data, img.Pix)

	pool, _ := app.shm.CreatePool(int(file.Fd()), size)
	defer pool.Destroy()

	buf, _ := pool.CreateBuffer(0, app.width, app.height, app.width*4, uint32(client.ShmFormatArgb8888))
	app.surface.Attach(buf, 0, 0)
	app.surface.Damage(0, 0, app.width, app.height)
	app.surface.Commit()
}

func createShmFile(size int) (*os.File, error) {
	path := fmt.Sprintf("/dev/shm/alloy-gui-%d", time.Now().UnixNano())
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	os.Remove(path)
	if err := file.Truncate(int64(size)); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}
