// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package media

import (
	"context"
	//"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/ServiceWeaver/weaver"
	"github.com/gin-gonic/gin"
	apimodel "github.com/superseriousbusiness/gotosocial/internal/api/model"
	apiutil "github.com/superseriousbusiness/gotosocial/internal/api/util"
	"github.com/superseriousbusiness/gotosocial/internal/config"
	"github.com/superseriousbusiness/gotosocial/internal/db/bundb"
	"github.com/superseriousbusiness/gotosocial/internal/gtserror"
	"github.com/superseriousbusiness/gotosocial/internal/media"
	"github.com/superseriousbusiness/gotosocial/internal/oauth"
	"github.com/superseriousbusiness/gotosocial/internal/processing"
	"github.com/superseriousbusiness/gotosocial/internal/state"
	gtsstorage "github.com/superseriousbusiness/gotosocial/internal/storage"
	"github.com/superseriousbusiness/gotosocial/internal/typeutils"
)

var requestHandler MediaRequestHandler

type MediaRequestHandler interface {
	Create(ctx context.Context, id string, form *apimodel.AttachmentRequest) (*apimodel.Attachment, error)
}

type mediaRequestHandler struct {
	weaver.Implements[MediaRequestHandler]
}

type app struct {
	weaver.Implements[weaver.Main]
	mediaHandler weaver.Ref[MediaRequestHandler]
}

func (r *mediaRequestHandler) Create(ctx context.Context, id string, form *apimodel.AttachmentRequest) (*apimodel.Attachment, error) {
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
	storage, err := gtsstorage.AutoConfig("hello.lock")
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

	apiAttachment, _ := processor.Media().Create(ctx, id, form)

	return apiAttachment, nil
}

func serve(ctx context.Context, app *app) error {
	requestHandler = app.mediaHandler.Get()
	return nil
}

// MediaCreatePOSTHandler swagger:operation POST /api/{api_version}/media mediaCreate
//
// Upload a new media attachment.
//
//	---
//	tags:
//	- media
//
//	consumes:
//	- multipart/form-data
//
//	produces:
//	- application/json
//
//	parameters:
//	-
//		name: api_version
//		type: string
//		in: path
//		description: Version of the API to use. Must be either `v1` or `v2`.
//		required: true
//	-
//		name: description
//		in: formData
//		description: >-
//			Image or media description to use as alt-text on the attachment.
//			This is very useful for users of screenreaders!
//			May or may not be required, depending on your instance settings.
//		type: string
//	-
//		name: focus
//		in: formData
//		description: >-
//			Focus of the media file.
//			If present, it should be in the form of two comma-separated floats between -1 and 1.
//			For example: `-0.5,0.25`.
//		type: string
//		default: "0,0"
//	-
//		name: file
//		in: formData
//		description: The media attachment to upload.
//		type: file
//		required: true
//
//	security:
//	- OAuth2 Bearer:
//		- write:media
//
//	responses:
//		'200':
//			description: The newly-created media attachment.
//			schema:
//				"$ref": "#/definitions/attachment"
//		'400':
//			description: bad request
//		'401':
//			description: unauthorized
//		'422':
//			description: unprocessable
//		'500':
//			description: internal server error
func (m *Module) MediaCreatePOSTHandler(c *gin.Context) {
	if err := weaver.Run(context.Background(), serve); err != nil {
		fmt.Printf("Unable to create service weaver component: %w\n", err)
	}
	apiVersion, errWithCode := apiutil.ParseAPIVersion(
		c.Param(apiutil.APIVersionKey),
		[]string{apiutil.APIv1, apiutil.APIv2}...,
	)
	if errWithCode != nil {
		apiutil.ErrorHandler(c, errWithCode, m.processor.InstanceGetV1)
		return
	}

	authed, err := oauth.Authed(c, true, true, true, true)
	if err != nil {
		apiutil.ErrorHandler(c, gtserror.NewErrorUnauthorized(err, err.Error()), m.processor.InstanceGetV1)
		return
	}

	if _, err := apiutil.NegotiateAccept(c, apiutil.JSONAcceptHeaders...); err != nil {
		apiutil.ErrorHandler(c, gtserror.NewErrorNotAcceptable(err, err.Error()), m.processor.InstanceGetV1)
		return
	}

	form := &apimodel.AttachmentRequest{}
	if err := c.ShouldBind(&form); err != nil {
		apiutil.ErrorHandler(c, gtserror.NewErrorBadRequest(err, err.Error()), m.processor.InstanceGetV1)
		return
	}

	if err := validateCreateMedia(form); err != nil {
		apiutil.ErrorHandler(c, gtserror.NewErrorBadRequest(err, err.Error()), m.processor.InstanceGetV1)
		return
	}

	apiAttachment, err := requestHandler.Create(c.Request.Context(), authed.Account.ID, form)
	if err != nil {
		apiutil.ErrorHandler(c, gtserror.NewErrorBadRequest(err, err.Error()), m.processor.InstanceGetV1)
		return
	}
	//apiAttachment, errWithCode := m.processor.Media().Create(c.Request.Context(), authed.Account, form)
	/*if errWithCode != nil {
		apiutil.ErrorHandler(c, errWithCode, m.processor.InstanceGetV1)
		return
	}*/

	if apiVersion == apiutil.APIv2 {
		// the mastodon v2 media API specifies that the URL should be null
		// and that the client should call /api/v1/media/:id to get the URL
		//
		// so even though we have the URL already, remove it now to comply
		// with the api
		apiAttachment.URL = nil
	}

	apiutil.JSON(c, http.StatusOK, apiAttachment)
}

func validateCreateMedia(form *apimodel.AttachmentRequest) error {
	// check there actually is a file attached and it's not size 0
	if form.File == nil {
		return errors.New("no attachment given")
	}

	maxVideoSize := config.GetMediaVideoMaxSize()
	maxImageSize := config.GetMediaImageMaxSize()
	minDescriptionChars := config.GetMediaDescriptionMinChars()
	maxDescriptionChars := config.GetMediaDescriptionMaxChars()

	// a very superficial check to see if no size limits are exceeded
	// we still don't actually know which media types we're dealing with but the other handlers will go into more detail there
	maxSize := maxVideoSize
	if maxImageSize > maxSize {
		maxSize = maxImageSize
	}

	if form.File.Size > int64(maxSize) {
		return fmt.Errorf("file size limit exceeded: limit is %d bytes but attachment was %d bytes", maxSize, form.File.Size)
	}

	if length := len([]rune(form.Description)); length > maxDescriptionChars {
		return fmt.Errorf("image description length must be between %d and %d characters (inclusive), but provided image description was %d chars", minDescriptionChars, maxDescriptionChars, length)
	}

	return nil
}
