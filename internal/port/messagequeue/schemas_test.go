package messagequeue

import (
	"encoding/json"
	"testing"
)

func TestConversationMessagePayloadWithImagesRoundTrip(t *testing.T) {
	t.Parallel()

	orig := ConversationMessagePayload{
		Role:    "user",
		Content: "Analyze this screenshot",
		Images: []MessageImagePayload{
			{
				Data:      "iVBORw0KGgo=",
				MediaType: "image/png",
				AltText:   "A screenshot",
			},
			{
				Data:      "/9j/4AAQ=",
				MediaType: "image/jpeg",
			},
		},
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded ConversationMessagePayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Role != orig.Role {
		t.Errorf("Role = %q, want %q", decoded.Role, orig.Role)
	}
	if decoded.Content != orig.Content {
		t.Errorf("Content = %q, want %q", decoded.Content, orig.Content)
	}
	if len(decoded.Images) != 2 {
		t.Fatalf("Images len = %d, want 2", len(decoded.Images))
	}
	if decoded.Images[0].Data != "iVBORw0KGgo=" {
		t.Errorf("Images[0].Data = %q, want %q", decoded.Images[0].Data, "iVBORw0KGgo=")
	}
	if decoded.Images[0].MediaType != "image/png" {
		t.Errorf("Images[0].MediaType = %q, want %q", decoded.Images[0].MediaType, "image/png")
	}
	if decoded.Images[0].AltText != "A screenshot" {
		t.Errorf("Images[0].AltText = %q, want %q", decoded.Images[0].AltText, "A screenshot")
	}
	if decoded.Images[1].MediaType != "image/jpeg" {
		t.Errorf("Images[1].MediaType = %q, want %q", decoded.Images[1].MediaType, "image/jpeg")
	}
	if decoded.Images[1].AltText != "" {
		t.Errorf("Images[1].AltText = %q, want empty", decoded.Images[1].AltText)
	}
}

func TestConversationMessagePayloadImagesOmittedWhenEmpty(t *testing.T) {
	t.Parallel()

	msg := ConversationMessagePayload{
		Role:    "assistant",
		Content: "Hello",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map: %v", err)
	}

	if _, found := raw["images"]; found {
		t.Error("nil Images should be omitted from JSON (omitempty)")
	}
}

func TestMessageImagePayloadRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		img  MessageImagePayload
	}{
		{
			name: "all fields",
			img: MessageImagePayload{
				Data:      "base64data",
				MediaType: "image/png",
				AltText:   "diagram",
			},
		},
		{
			name: "no alt_text",
			img: MessageImagePayload{
				Data:      "AAAA",
				MediaType: "image/webp",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(tt.img)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}

			var decoded MessageImagePayload
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			if decoded.Data != tt.img.Data {
				t.Errorf("Data = %q, want %q", decoded.Data, tt.img.Data)
			}
			if decoded.MediaType != tt.img.MediaType {
				t.Errorf("MediaType = %q, want %q", decoded.MediaType, tt.img.MediaType)
			}
			if decoded.AltText != tt.img.AltText {
				t.Errorf("AltText = %q, want %q", decoded.AltText, tt.img.AltText)
			}

			if tt.img.AltText == "" {
				var raw map[string]any
				if err := json.Unmarshal(data, &raw); err != nil {
					t.Fatalf("Unmarshal to map: %v", err)
				}
				if _, found := raw["alt_text"]; found {
					t.Error("empty AltText should be omitted from JSON (omitempty)")
				}
			}
		})
	}
}
