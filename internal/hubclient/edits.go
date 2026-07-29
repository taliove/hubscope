package hubclient

import (
	"bytes"
	"context"
	_ "embed"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"sort"
	"strings"
)

// probeImagePNG is the fixed test image uploaded on every images_edit probe
// (spec 0014 / GH #32): a 1024x1024 solid teal PNG, embedded so probing the
// edit path never depends on external assets and the single-binary delivery
// (W8) stays intact. The content is a fixed asset (not generated at runtime)
// so it can be eyeballed and stays byte-stable across releases.
//
//go:embed assets/probe-image.png
var probeImagePNG []byte

// probeImageFilename is the multipart file name sent for the test image.
const probeImageFilename = "probe-image.png"

// editProbePrompt is the fixed edit instruction sent on every images_edit
// probe. Each call produces a real edited image and costs money (spec 0014),
// so the instruction stays trivial.
const editProbePrompt = "Make the teal square slightly darker"

// callImagesEdit executes one POST /v1/images/edits call as a multipart form
// with the OpenAI contract fields: image (the embedded test image), prompt,
// model — singular field names, no image[]/file dialects — plus any
// rule-merged extra parameters as plain form fields (spec 0014 / GH #33, the
// same merged map generations puts into its JSON body). Success
// determination and usage mapping are identical to generations (see
// doImageCall); there is no streaming mode and no TTFT.
func (c *Client) callImagesEdit(ctx context.Context, baseURL, token, modelID string, imageParams map[string]string) Result {
	url := strings.TrimRight(baseURL, "/") + "/v1/images/edits"

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	// CreatePart with an explicit header keeps the part's Content-Type at
	// image/png; CreateFormFile would label it application/octet-stream,
	// which some upstreams reject.
	partHeader := textproto.MIMEHeader{}
	partHeader.Set("Content-Disposition", `form-data; name="image"; filename="`+probeImageFilename+`"`)
	partHeader.Set("Content-Type", "image/png")
	part, err := w.CreatePart(partHeader)
	if err != nil {
		msg := truncate("build multipart: " + err.Error())
		return Result{OK: false, HTTPStatus: 0, ErrorSummary: &msg}
	}
	if _, err := part.Write(probeImagePNG); err != nil {
		msg := truncate("build multipart: " + err.Error())
		return Result{OK: false, HTTPStatus: 0, ErrorSummary: &msg}
	}
	for _, field := range [][2]string{{"prompt", editProbePrompt}, {"model", modelID}} {
		if err := w.WriteField(field[0], field[1]); err != nil {
			msg := truncate("build multipart: " + err.Error())
			return Result{OK: false, HTTPStatus: 0, ErrorSummary: &msg}
		}
	}
	// Rule-merged extra params ride as plain form fields, in sorted key order
	// so the wire shape stays deterministic across calls.
	extraKeys := make([]string, 0, len(imageParams))
	for k := range imageParams {
		extraKeys = append(extraKeys, k)
	}
	sort.Strings(extraKeys)
	for _, k := range extraKeys {
		if err := w.WriteField(k, imageParams[k]); err != nil {
			msg := truncate("build multipart: " + err.Error())
			return Result{OK: false, HTTPStatus: 0, ErrorSummary: &msg}
		}
	}
	if err := w.Close(); err != nil {
		msg := truncate("build multipart: " + err.Error())
		return Result{OK: false, HTTPStatus: 0, ErrorSummary: &msg}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		msg := truncate("build request: " + err.Error())
		return Result{OK: false, HTTPStatus: 0, ErrorSummary: &msg}
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)

	return c.doImageCall(req)
}
