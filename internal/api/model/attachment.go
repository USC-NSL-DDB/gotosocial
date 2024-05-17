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

package model


import (
	"bytes"
	"encoding/binary"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"strings"
	"github.com/ServiceWeaver/weaver"
)


// AttachmentRequest models media attachment creation parameters.
//
// swagger: ignore
type AttachmentRequest struct {
	File *multipart.FileHeader `form:"file" binding:"required"`
	// Description of the media file. Optional.
	// This will be used as alt-text for users of screenreaders etc.
	// example: This is an image of some kittens, they are very cute and fluffy.
	Description string `form:"description"`
	// Focus of the media file. Optional.
	// If present, it should be in the form of two comma-separated floats between -1 and 1.
	// example: -0.5,0.565
	Focus string `form:"focus"`
}

// MarshalBinary implements the encoding.BinaryMarshaler interface.
func (ar *AttachmentRequest) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer

	// Marshal the File field
	if ar.File != nil {
		fileData, err := MarshalBinaryMultipart(ar.File)
		if err != nil {
			return nil, err
		}
		// Write the length of fileData and the fileData itself
		if err := binary.Write(&buf, binary.LittleEndian, int64(len(fileData))); err != nil {
			return nil, err
		}
		if _, err := buf.Write(fileData); err != nil {
			return nil, err
		}
	} else {
		// Write a length of -1 to indicate a nil File field
		if err := binary.Write(&buf, binary.LittleEndian, int64(-1)); err != nil {
			return nil, err
		}
	}

	// Marshal the Description field
	if err := writeString(&buf, ar.Description); err != nil {
		return nil, err
	}

	// Marshal the Focus field
	if err := writeString(&buf, ar.Focus); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// UnmarshalBinary implements the encoding.BinaryUnmarshaler interface.
func (ar *AttachmentRequest) UnmarshalBinary(data []byte) error {
	buf := bytes.NewReader(data)

	// Unmarshal the File field
	var fileDataLen int64
	if err := binary.Read(buf, binary.LittleEndian, &fileDataLen); err != nil {
		return err
	}
	if fileDataLen != -1 {
		fileData := make([]byte, fileDataLen)
		if _, err := buf.Read(fileData); err != nil {
			return err
		}
		file, err := UnmarshalBinaryMultipart(fileData)
		if err != nil {
			return err
		}
		ar.File = file
	} else {
		ar.File = nil
	}

	// Unmarshal the Description field
	description, err := readString(buf)
	if err != nil {
		return err
	}
	ar.Description = description

	// Unmarshal the Focus field
	focus, err := readString(buf)
	if err != nil {
		return err
	}
	ar.Focus = focus

	return nil
}

// Helper function to write a string with its length prefix
func writeString(buf *bytes.Buffer, str string) error {
	if err := binary.Write(buf, binary.LittleEndian, int64(len(str))); err != nil {
		return err
	}
	if _, err := buf.WriteString(str); err != nil {
		return err
	}
	return nil
}

// Helper function to read a string with its length prefix
func readString(buf *bytes.Reader) (string, error) {
	var strLen int64
	if err := binary.Read(buf, binary.LittleEndian, &strLen); err != nil {
		return "", err
	}
	str := make([]byte, strLen)
	if _, err := buf.Read(str); err != nil {
		return "", err
	}
	return string(str), nil
}

// MarshalBinary implements the encoding.BinaryMarshaler interface for multipart.FileHeader.
func MarshalBinaryMultipart(fh *multipart.FileHeader) ([]byte, error) {
	var buf bytes.Buffer

	// Write the filename length and filename
	filename := fh.Filename
	if err := binary.Write(&buf, binary.LittleEndian, int64(len(filename))); err != nil {
		return nil, err
	}
	if _, err := buf.WriteString(filename); err != nil {
		return nil, err
	}

	// Write the header length and header
	header := fh.Header
	headerStr := headerToString(header)
	if err := binary.Write(&buf, binary.LittleEndian, int64(len(headerStr))); err != nil {
		return nil, err
	}
	if _, err := buf.WriteString(headerStr); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// UnmarshalBinary implements the encoding.BinaryUnmarshaler interface for multipart.FileHeader.
func UnmarshalBinaryMultipart(data []byte) (*multipart.FileHeader, error) {
	fh := multipart.FileHeader{}
	buf := bytes.NewReader(data)

	// Read the filename length and filename
	var filenameLen int64
	if err := binary.Read(buf, binary.LittleEndian, &filenameLen); err != nil {
		return nil,err
	}
	filename := make([]byte, filenameLen)
	if _, err := buf.Read(filename); err != nil {
		return nil, err
	}
	fh.Filename = string(filename)

	// Read the header length and header
	var headerLen int64
	if err := binary.Read(buf, binary.LittleEndian, &headerLen); err != nil {
		return nil, err
	}
	header := make([]byte, headerLen)
	if _, err := buf.Read(header); err != nil {
		return nil, err
	}
	fh.Header = stringToHeader(string(header))

	return &fh, nil
}

// Helper function to convert a textproto.MIMEHeader to a string
func headerToString(header textproto.MIMEHeader) string {
	var sb strings.Builder
	for key, values := range header {
		for _, value := range values {
			sb.WriteString(fmt.Sprintf("%s: %s\n", key, value))
		}
	}
	return sb.String()
}

// Helper function to convert a string to a textproto.MIMEHeader
func stringToHeader(headerStr string) textproto.MIMEHeader {
	header := textproto.MIMEHeader{}
	lines := strings.Split(headerStr, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) == 2 {
			header.Add(parts[0], parts[1])
		}
	}
	return header
}


// AttachmentUpdateRequest models an update request for an attachment.
//
// swagger:ignore
type AttachmentUpdateRequest struct {
	weaver.AutoMarshal
	// Description of the media file.
	// This will be used as alt-text for users of screenreaders etc.
	// allowEmptyValue: true
	Description *string `form:"description" json:"description" xml:"description"`
	// Focus of the media file.
	// If present, it should be in the form of two comma-separated floats between -1 and 1.
	// allowEmptyValue: true
	Focus *string `form:"focus" json:"focus" xml:"focus"`
}

// Attachment models a media attachment.
//
// swagger:model attachment
type Attachment struct {
	weaver.AutoMarshal
	// The ID of the attachment.
	// example: 01FC31DZT1AYWDZ8XTCRWRBYRK
	ID string `json:"id"`
	// The type of the attachment.
	// enum:
	//   - unknown
	//   - image
	//   - gifv
	//   - video
	//   - audio
	// example: image
	Type string `json:"type"`
	// The location of the original full-size attachment.
	// example: https://example.org/fileserver/some_id/attachments/some_id/original/attachment.jpeg
	URL *string `json:"url"`
	// A shorter URL for the attachment.
	// In our case, we just give the URL again since we don't create smaller URLs.
	TextURL *string `json:"text_url"`
	// The location of a scaled-down preview of the attachment.
	// example: https://example.org/fileserver/some_id/attachments/some_id/small/attachment.jpeg
	PreviewURL *string `json:"preview_url"`
	// The location of the full-size original attachment on the remote server.
	// Only defined for instances other than our own.
	// example: https://some-other-server.org/attachments/original/ahhhhh.jpeg
	RemoteURL *string `json:"remote_url"`
	// The location of a scaled-down preview of the attachment on the remote server.
	// Only defined for instances other than our own.
	// example: https://some-other-server.org/attachments/small/ahhhhh.jpeg
	PreviewRemoteURL *string `json:"preview_remote_url"`
	// Metadata for this attachment.
	Meta *MediaMeta `json:"meta"`
	// Alt text that describes what is in the media attachment.
	// example: This is a picture of a kitten.
	Description *string `json:"description"`
	// A hash computed by the BlurHash algorithm, for generating colorful preview thumbnails when media has not been downloaded yet.
	// See https://github.com/woltapp/blurhash
	Blurhash *string `json:"blurhash"`

	// Additional fields not exposed via JSON
	// (used only internally for templating etc).

	// Parent status of this media is sensitive.
	Sensitive bool `json:"-"`
}

// MediaMeta models media metadata.
// This can be metadata about an image, an audio file, video, etc.
//
// swagger:model mediaMeta
type MediaMeta struct {
	weaver.AutoMarshal
	// Dimensions of the original media.
	Original MediaDimensions `json:"original"`
	// Dimensions of the thumbnail/small version of the media.
	Small MediaDimensions `json:"small,omitempty"`
	// Focus data for the media.
	Focus *MediaFocus `json:"focus,omitempty"`
}

// MediaFocus models the focal point of a piece of media.
//
// swagger:model mediaFocus
type MediaFocus struct {
	weaver.AutoMarshal
	// x position of the focus
	// should be between -1 and 1
	X float32 `json:"x"`
	// y position of the focus
	// should be between -1 and 1
	Y float32 `json:"y"`
}

// MediaDimensions models detailed properties of a piece of media.
//
// swagger:model mediaDimensions
type MediaDimensions struct {
	weaver.AutoMarshal
	// Width of the media in pixels.
	// Not set for audio.
	// example: 1920
	Width int `json:"width,omitempty"`
	// Height of the media in pixels.
	// Not set for audio.
	// example: 1080
	Height int `json:"height,omitempty"`
	// Framerate of the media.
	// Only set for video and gifs.
	// example: 30
	FrameRate string `json:"frame_rate,omitempty"`
	// Duration of the media in seconds.
	// Only set for video and audio.
	// example: 5.43
	Duration float32 `json:"duration,omitempty"`
	// Bitrate of the media in bits per second.
	// example: 1000000
	Bitrate int `json:"bitrate,omitempty"`
	// Size of the media, in the format `[width]x[height]`.
	// Not set for audio.
	// example: 1920x1080
	Size string `json:"size,omitempty"`
	// Aspect ratio of the media.
	// Equal to width / height.
	// example: 1.777777778
	Aspect float32 `json:"aspect,omitempty"`
}
