package weaver

import (
	"context"
	"fmt"
	"time"

	"github.com/ServiceWeaver/weaver"
	apimodel "github.com/superseriousbusiness/gotosocial/internal/api/model"
	"github.com/superseriousbusiness/gotosocial/internal/db/bundb"
	"github.com/superseriousbusiness/gotosocial/internal/media"
	"github.com/superseriousbusiness/gotosocial/internal/processing"
	"github.com/superseriousbusiness/gotosocial/internal/state"
	gtsstorage "github.com/superseriousbusiness/gotosocial/internal/storage"
	"github.com/superseriousbusiness/gotosocial/internal/typeutils"
)

type MediaRequestOperation int

const (
	CREATE_MEDIA MediaRequestOperation = 0
	UPDATE_MEDIA MediaRequestOperation = 1
	GET_MEDIA    MediaRequestOperation = 2
)

type MediaRequestHandler interface {
	DoOperation(ctx context.Context, id string, form *apimodel.AttachmentRequest, attachmentID string, formUpdate *apimodel.AttachmentUpdateRequest, op MediaRequestOperation) (*apimodel.Attachment, error)
}

type mediaRequestHandler struct {
	weaver.Implements[MediaRequestHandler]
}

func (r *mediaRequestHandler) DoOperation(ctx context.Context, id string, form *apimodel.AttachmentRequest, attachmentID string, formUpdate *apimodel.AttachmentUpdateRequest, op MediaRequestOperation) (*apimodel.Attachment, error) {
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

	// Build handlers used in later initializations.
	mediaManager := media.NewManager(&state)
	typeConverter := typeutils.NewConverter(&state)
	processor := processing.NewProcessorWithMedia(typeConverter, mediaManager, &state)

	var apiAttachment *apimodel.Attachment

	switch op {
	case CREATE_MEDIA:
		apiAttachment, _ = processor.Media().Create(ctx, id, form)
	case UPDATE_MEDIA:
		apiAttachment, _ = processor.Media().Update(ctx, id, attachmentID, formUpdate)
	case GET_MEDIA:
		apiAttachment, _ = processor.Media().Get(ctx, id, attachmentID)
	default:
		return nil, fmt.Errorf("invalid media operation")
	}

	return apiAttachment, nil
}
