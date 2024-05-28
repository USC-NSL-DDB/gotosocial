package weaver

import (
	"context"
	"fmt"
	"time"

	"github.com/ServiceWeaver/weaver"
	apimodel "github.com/superseriousbusiness/gotosocial/internal/api/model"
	"github.com/superseriousbusiness/gotosocial/internal/cleaner"
	"github.com/superseriousbusiness/gotosocial/internal/config"
	"github.com/superseriousbusiness/gotosocial/internal/db/bundb"
	"github.com/superseriousbusiness/gotosocial/internal/federation"
	"github.com/superseriousbusiness/gotosocial/internal/federation/federatingdb"
	"github.com/superseriousbusiness/gotosocial/internal/filter/spam"
	"github.com/superseriousbusiness/gotosocial/internal/filter/visibility"
	"github.com/superseriousbusiness/gotosocial/internal/gtsmodel"
	"github.com/superseriousbusiness/gotosocial/internal/httpclient"
	"github.com/superseriousbusiness/gotosocial/internal/media"
	"github.com/superseriousbusiness/gotosocial/internal/oauth"
	"github.com/superseriousbusiness/gotosocial/internal/processing"
	"github.com/superseriousbusiness/gotosocial/internal/state"
	gtsstorage "github.com/superseriousbusiness/gotosocial/internal/storage"
	"github.com/superseriousbusiness/gotosocial/internal/transport"
	"github.com/superseriousbusiness/gotosocial/internal/typeutils"
)

type StatusRequestHandler interface {
	DoOperation(
		ctx context.Context,
		requester *gtsmodel.Account,
		application *gtsmodel.Application,
		form *apimodel.AdvancedStatusCreateForm,
	) (
		*apimodel.Status,
		error,
	)
}

type statusRequestHandler struct {
	weaver.Implements[StatusRequestHandler]
}

func (r *statusRequestHandler) DoOperation(
	ctx context.Context,
	requester *gtsmodel.Account,
	application *gtsmodel.Application,
	form *apimodel.AdvancedStatusCreateForm,
) (
	*apimodel.Status,
	error,
) {

	var state state.State

	// Initialize caches
	state.Caches.Init()
	state.Caches.Start()
	defer state.Caches.Stop()

	// Open connection to the database
	dbService, err := bundb.NewBunDBService(ctx, &state)
	if err != nil {
		return nil, fmt.Errorf("error creating dbservice: %s", err)
	}

	// Set the state DB connection
	state.DB = dbService

	if err := dbService.CreateInstanceAccount(ctx); err != nil {
		return nil, fmt.Errorf("error creating instance account: %s", err)
	}

	if err := dbService.CreateInstanceInstance(ctx); err != nil {
		return nil, fmt.Errorf("error creating instance instance: %s", err)
	}

	// Open the storage backend
	storage, err := gtsstorage.AutoConfig(typeutils.RandStringRunes(5) + ".lock")
	if err != nil {
		return nil, fmt.Errorf("error creating storage backend: %w", err)
	}

	// Set the state storage driver
	state.Storage = storage

	// Build HTTP client
	client := httpclient.New(httpclient.Config{
		AllowRanges:           config.MustParseIPPrefixes(config.GetHTTPClientAllowIPs()),
		BlockRanges:           config.MustParseIPPrefixes(config.GetHTTPClientBlockIPs()),
		Timeout:               config.GetHTTPClientTimeout(),
		TLSInsecureSkipVerify: config.GetHTTPClientTLSInsecureSkipVerify(),
	})

	// Initialize workers.
	state.Workers.Start()
	defer state.Workers.Stop()

	// Add a task to the scheduler to sweep caches.
	// Frequency = 1 * minute
	// Threshold = 80% capacity
	_ = state.Workers.Scheduler.AddRecurring(
		"@cachesweep", // id
		time.Time{},   // start
		time.Minute,   // freq
		func(context.Context, time.Time) {
			state.Caches.Sweep(60)
		},
	)
	fmt.Printf("----state %+v\n", state)

	// Build handlers used in later initializations.
	typeConverter := typeutils.NewConverter(&state)
	mediaManager := media.NewManager(&state)
	oauthServer := oauth.New(ctx, dbService)
	visFilter := visibility.NewFilter(&state)
	spamFilter := spam.NewFilter(&state)
	federatingDB := federatingdb.New(&state, typeConverter, visFilter, spamFilter)
	transportController := transport.NewController(&state, federatingDB, &federation.Clock{}, client)
	federator := federation.NewFederator(&state, federatingDB, transportController, typeConverter, visFilter, mediaManager)
	// Create a media cleaner using the given state.
	cleaner := cleaner.New(&state)

	// Create the processor using all the other services we've created so far.
	processor := processing.NewProcessor(
		cleaner,
		typeConverter,
		federator,
		oauthServer,
		mediaManager,
		&state,
		nil,
	)

	// Set state client / federator asynchronous worker enqueue functions
	state.Workers.EnqueueClientAPI = processor.Workers().EnqueueClientAPI
	state.Workers.EnqueueFediAPI = processor.Workers().EnqueueFediAPI

	// Set state client / federator synchronous processing functions.
	state.Workers.ProcessFromClientAPI = processor.Workers().ProcessFromClientAPI
	state.Workers.ProcessFromFediAPI = processor.Workers().ProcessFromFediAPI

	apiStatus, _ := processor.Status().Create(
		ctx,
		requester,
		application,
		form,
	)

	return apiStatus, nil
}
