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
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"

	apimodel "github.com/superseriousbusiness/gotosocial/internal/api/model"
	"github.com/superseriousbusiness/gotosocial/internal/gtserror"
	"github.com/superseriousbusiness/gotosocial/internal/gtsmodel"
	"github.com/superseriousbusiness/gotosocial/internal/media"
)

const TmpMedia = "/tmp/gotosocial/"

func createMultipartFileHeader(filePath string) *multipart.FileHeader {
	// open the file
	file, err := os.Open(filePath)
	if err != nil {
		log.Fatal(err)
		return nil
	}
	defer file.Close()

	// create a buffer to hold the file in memory
	var buff bytes.Buffer
	buffWriter := io.Writer(&buff)

	// create a new form and create a new file field
	formWriter := multipart.NewWriter(buffWriter)
	formPart, err := formWriter.CreateFormFile("file", filepath.Base(file.Name()))
	if err != nil {
		log.Fatal(err)
		return nil
	}

	// copy the content of the file to the form's file field
	if _, err := io.Copy(formPart, file); err != nil {
		log.Fatal(err)
		return nil
	}

	// close the form writer after the copying process is finished
	// I don't use defer in here to avoid unexpected EOF error
	formWriter.Close()

	// transform the bytes buffer into a form reader
	buffReader := bytes.NewReader(buff.Bytes())
	formReader := multipart.NewReader(buffReader, formWriter.Boundary())

	// read the form components with max stored memory of 1MB
	multipartForm, err := formReader.ReadForm(1 << 20)
	if err != nil {
		log.Fatal(err)
		return nil
	}

	// return the multipart file header
	files, exists := multipartForm.File["file"]
	if !exists || len(files) == 0 {
		log.Fatal("multipart file not exists")
		return nil
	}

	return files[0]
}

// Create creates a new media attachment belonging to the given account, using the request form.
func (p *Processor) Create(ctx context.Context, id string, form *apimodel.AttachmentRequest) (*apimodel.Attachment, gtserror.WithCode) {
	fileMultiPart := createMultipartFileHeader(filepath.Join(TmpMedia, form.File.Filename))
	form.File = fileMultiPart
	data := func(innerCtx context.Context) (io.ReadCloser, int64, error) {
		f, err := form.File.Open()
		return f, form.File.Size, err
	}

	focusX, focusY, err := parseFocus(form.Focus)
	if err != nil {
		err := fmt.Errorf("could not parse focus value %s: %s", form.Focus, err)
		return nil, gtserror.NewErrorBadRequest(err, err.Error())
	}

	// process the media attachment and load it immediately
	media := p.mediaManager.PreProcessMedia(data, id, &media.AdditionalMediaInfo{
		Description: &form.Description,
		FocusX:      &focusX,
		FocusY:      &focusY,
	})

	attachment, err := media.LoadAttachment(ctx)
	if err != nil {
		return nil, gtserror.NewErrorUnprocessableEntity(err, err.Error())
	} else if attachment.Type == gtsmodel.FileTypeUnknown {
		err = gtserror.Newf("could not process uploaded file with extension %s", attachment.File.ContentType)
		return nil, gtserror.NewErrorUnprocessableEntity(err, err.Error())
	}

	apiAttachment, err := p.converter.AttachmentToAPIAttachment(ctx, attachment)
	if err != nil {
		err := fmt.Errorf("error parsing media attachment to frontend type: %s", err)
		return nil, gtserror.NewErrorInternalError(err)
	}

	return &apiAttachment, nil
}
