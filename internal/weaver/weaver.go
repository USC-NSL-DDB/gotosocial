package weaver

import (
	"context"
	"fmt"

	"github.com/ServiceWeaver/weaver"
)

type App struct {
	weaver.Implements[weaver.Main]
	mediaHandler weaver.Ref[MediaRequestHandler]
	gotosocial   weaver.Listener
}

type AppContext struct {
	MediaRequestHandler   MediaRequestHandler
	ServiceWeaverListener *weaver.Listener
}

func NewServiceWeaverContext() *AppContext {
	app := AppContext{}
	app.Main()
	return &app
}

func (a *AppContext) CreateRequestHandlers(ctx context.Context, app *App) error {
	a.MediaRequestHandler = app.mediaHandler.Get()
	a.ServiceWeaverListener = &app.gotosocial
	return nil
}

func (a *AppContext) Main() {
	if err := weaver.Run(context.Background(), a.CreateRequestHandlers); err != nil {
		fmt.Printf("Unable to create service weaver component: %s\n", err)
	}
}
