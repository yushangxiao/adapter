package v1

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"github.com/bincooo/chatgpt-adapter/v2/pkg"
	"github.com/bincooo/emit.io"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/sha3"
	"math/rand"
	"net/http"
	"time"
)

const (
	ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36"
)

var (
	CORE_COUNTS        = []int{8, 12, 16, 24}
	SCREEN_RESOLUTIONS = []int{3000, 4000, 6000}
	HASH_ATTEMPTS      = 100000
	BASE64_PREFIX      = "gAAAAAB"
)

type chatSession struct {
	Persona string `json:"persona"`
	Pow     struct {
		Required bool   `json:"required"`
		Seed     string `json:"seed"`
		Diff     string `json:"difficulty"`
	} `json:"proofofwork"`
	Token    string `json:"token"`
	deviceId string
}

type chatSSEResponse struct {
	Message struct {
		Id     string `json:"id"`
		Author struct {
			Role     string      `json:"role"`
			Name     interface{} `json:"name"`
			Metadata struct {
			} `json:"metadata"`
		} `json:"author"`
		CreateTime float64     `json:"create_time"`
		UpdateTime interface{} `json:"update_time"`
		Content    struct {
			ContentType string   `json:"content_type"`
			Parts       []string `json:"parts"`
		} `json:"content"`
		Status   string      `json:"status"`
		EndTurn  interface{} `json:"end_turn"`
		Weight   float64     `json:"weight"`
		Metadata struct {
			Citations         []interface{} `json:"citations"`
			GizmoId           interface{}   `json:"gizmo_id"`
			MessageType       string        `json:"message_type"`
			ModelSlug         string        `json:"model_slug"`
			DefaultModelSlug  string        `json:"default_model_slug"`
			Pad               string        `json:"pad"`
			ParentId          string        `json:"parent_id"`
			ModelSwitcherDeny []interface{} `json:"model_switcher_deny"`
		} `json:"metadata"`
		Recipient string `json:"recipient"`
	} `json:"message"`
	ConversationId string      `json:"conversation_id"`
	Error          interface{} `json:"error"`
}

// reference https://github.com/hominsu/freegpt35
func fetchGpt35(ctx *gin.Context, req pkg.ChatCompletion) (*http.Response, error) {
	proxies := ctx.GetString("proxies")
	session, err := partOne(ctx.Request.Context(), proxies)
	if err != nil {
		return nil, err
	}
	return partTwo(ctx, session, req)
}

func partOne(ctx context.Context, proxies string) (*chatSession, error) {
	retry := 3
label:
	deviceId := uuid.NewString()
	response, err := emit.ClientBuilder().
		Context(ctx).
		Proxies(proxies).
		POST("https://chat.openai.com/backend-api/sentinel/chat-requirements").
		JHeader().
		Header("oai-device-id", deviceId).
		Header("oai-language", "en-US").
		Header("origin", "https://chat.openai.com").
		Header("referer", "https://chat.openai.com").
		Header("user-agent", ua).
		DoC(emit.Status(http.StatusOK), emit.IsJSON)
	if err != nil {
		if retry > 0 {
			retry--
			goto label
		}
		return nil, err
	}

	var session chatSession
	if err = emit.ToObject(response, &session); err != nil {
		return nil, err
	}

	session.deviceId = deviceId
	return &session, nil
}

func partTwo(ctx *gin.Context, session *chatSession, req pkg.ChatCompletion) (*http.Response, error) {
	proxies := ctx.GetString("proxies")
	return emit.ClientBuilder().
		Context(ctx.Request.Context()).
		Proxies(proxies).
		POST("https://chat.openai.com/backend-api/conversation").
		JHeader().
		Header("oai-device-id", session.deviceId).
		Header("openai-sentinel-chat-requirements-token", session.Token).
		Header("openai-sentinel-proof-token", generateToken(session.Pow.Seed, session.Pow.Diff)).
		Header("oai-language", "en-US").
		Header("origin", "https://chat.openai.com").
		Header("referer", "https://chat.openai.com").
		Header("user-agent", ua).
		Body(makePayload(req)).
		DoC(emit.Status(http.StatusOK), emit.IsSTREAM)
}

func makePayload(req pkg.ChatCompletion) map[string]interface{} {
	var messages []interface{}

	for _, message := range req.Messages {
		messages = append(messages, map[string]interface{}{
			"id": uuid.NewString(),
			"author": map[string]string{
				"role": message.GetString("role"),
			},
			"content": map[string]interface{}{
				"content_type": "text",
				"parts": []string{
					message.GetString("content"),
				},
			},
		})
	}

	obj := map[string]interface{}{
		"action":                        "next",
		"messages":                      messages,
		"parent_message_id":             uuid.NewString(),
		"model":                         req.Model,
		"timezone_offset_min":           -180,
		"suggestions":                   make([]string, 0),
		"history_and_training_disabled": true,
		"conversation_mode": map[string]string{
			"kind": "primary_assistant",
		},
		//"force_paragen_model_slug": "",
		//"force_paragen":            false,
		//"force_nulligen":           false,
		//"force_rate_limit":         false,
		"reset_rate_limits":    true,
		"websocket_request_id": uuid.NewString(),
	}
	return obj
}

func generateToken(seed, diff string) string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	core := CORE_COUNTS[r.Intn(len(CORE_COUNTS)-1)]
	screen := SCREEN_RESOLUTIONS[r.Intn(len(SCREEN_RESOLUTIONS)-1)]
	timeLayout := "Mon Jan 2 2006 15:04:05 GMT-0700 MST (MST)"
	timeStr := time.Now().Format(timeLayout)
	config := []interface{}{
		core + screen,
		timeStr,
		4294705152,
		0,
		ua,
	}
	l := len(diff) / 2
	h := sha3.New512()
	for i := 0; i < HASH_ATTEMPTS; i++ {
		config[3] = i
		marshal, _ := json.Marshal(config)
		b64 := base64.StdEncoding.EncodeToString(marshal)
		h.Write([]byte(seed + b64))
		hash := h.Sum(nil)
		h.Reset()
		if hex.EncodeToString(hash[:l]) <= diff {
			return BASE64_PREFIX + b64
		}
	}
	return BASE64_PREFIX + "wQ8Lk5FbGpA2NcR9dShT6gYjU7VxZ4D" + base64.StdEncoding.EncodeToString([]byte(`"`+seed+`"`))
}
