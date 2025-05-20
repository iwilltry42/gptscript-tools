package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/obot-platform/tools/openai-model-provider/api"
	"github.com/obot-platform/tools/openai-model-provider/proxy"
)

// KubeAIModel is basically the same as the api.Model, but with an additional Features field used to determine the model usage
type KubeAIModel struct {
	api.Model
	Features []string `json:"features,omitempty"`
}

type KubeAIModelsResponse struct {
	Object string        `json:"object"`
	Data   []KubeAIModel `json:"data"`
}

func cleanURL(endpoint string) string {
	return strings.TrimRight(endpoint, "/")
}

func main() {

	endpoint := os.Getenv("OBOT_KUBEAI_MODEL_PROVIDER_ENDPOINT")
	if endpoint == "" {
		fmt.Println("OBOT_KUBEAI_MODEL_PROVIDER_ENDPOINT environment variable not set, credential must be provided on a per-request basis")
	}

	endpoint = cleanURL(endpoint)
	u, err := url.Parse(endpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid endpoint URL %q: %v\n", endpoint, err)
		os.Exit(1)
	}

	if u.Scheme == "" {
		if u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || strings.HasSuffix(u.Hostname(), ".local") {
			u.Scheme = "http"
		} else {
			u.Scheme = "https"
		}
	}

	cfg := &proxy.Config{
		PersonalBaseURLHeader: "X-Obot-OBOT_KUBEAI_MODEL_PROVIDER_ENDPOINT",
		ListenPort:            os.Getenv("PORT"),
		BaseURL:               strings.TrimSuffix(u.String(), "/v1") + "/v1",
		RewriteModelsFn:       rewriteKubeAIModels,
		Name:                  "KubeAI",
	}

	if len(os.Args) > 1 && os.Args[1] == "validate" {
		if err := cfg.Validate("/tools/kubeai-model-provider/validate"); err != nil {
			os.Exit(1)
		}
		return
	}

	if err := proxy.Run(cfg); err != nil {
		panic(err)
	}
}

// rewriteKubeAIModels rewrites the models response to set usage based on the Features list
func rewriteKubeAIModels(resp *http.Response) error {
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	defer resp.Body.Close()

	var body io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzReader.Close()
		resp.Header.Del("Content-Encoding")
		body = gzReader
	}

	// KubeAI specific (incl. Features)
	var kubeModels KubeAIModelsResponse
	if err := json.NewDecoder(body).Decode(&kubeModels); err != nil {
		return fmt.Errorf("failed to decode models response: %w", err)
	}

	// OpenAI API format
	models := api.ModelsResponse{
		Object: kubeModels.Object,
		Data:   make([]api.Model, len(kubeModels.Data)),
	}

	for i, kubeModel := range kubeModels.Data {
		model := api.Model{
			ID:      kubeModel.ID,
			Object:  kubeModel.Object,
			Created: kubeModel.Created,
			OwnedBy: kubeModel.OwnedBy,
		}

		if kubeModel.Metadata == nil {
			model.Metadata = make(map[string]string)
		} else {
			model.Metadata = kubeModel.Metadata
		}

		model.Metadata["usage"] = "llm"

		// map usage based on first feature in KubeAI's features list
		// as of 2025-05-21, KubeAI only supports TextGeneration, TextEmbedding and SpeechToText (which is not supported by Obot right now)
		ok := false
		for _, feature := range kubeModel.Features {
			switch strings.ToLower(feature) {
			case "textgeneration":
				model.Metadata["usage"] = "llm"
				ok = true
			case "textembedding":
				model.Metadata["usage"] = "text-embedding"
				ok = true
			}
			if ok {
				break
			}
		}

		models.Data[i] = model
	}

	b, err := json.Marshal(models)
	if err != nil {
		return fmt.Errorf("failed to marshal models response: %w", err)
	}

	resp.Body = io.NopCloser(bytes.NewReader(b))
	resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(b)))
	return nil
}
