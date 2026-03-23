package proxy

import (
	"context"
	"net/http"
	"strings"
)

type ProtocolFamily string

const (
	ProtocolFamilyClaude ProtocolFamily = "claude"
	ProtocolFamilyOpenAI ProtocolFamily = "openai"
	ProtocolFamilyGemini ProtocolFamily = "gemini"
)

type RequestCapability string

const (
	CapabilityClaudeCompatible         RequestCapability = "claude_compatible"
	CapabilityClaudeMessages           RequestCapability = "claude_messages"
	CapabilityClaudeCountTokens        RequestCapability = "claude_count_tokens"
	CapabilityOpenAICompatible         RequestCapability = "openai_compatible"
	CapabilityOpenAIChatCompletions    RequestCapability = "openai_chat_completions"
	CapabilityOpenAICompletions        RequestCapability = "openai_completions"
	CapabilityOpenAIResponses          RequestCapability = "openai_responses"
	CapabilityOpenAIEmbeddings         RequestCapability = "openai_embeddings"
	CapabilityOpenAIModerations        RequestCapability = "openai_moderations"
	CapabilityOpenAIAudio              RequestCapability = "openai_audio"
	CapabilityOpenAIImages             RequestCapability = "openai_images"
	CapabilityOpenAIFiles              RequestCapability = "openai_files"
	CapabilityOpenAIUploads            RequestCapability = "openai_uploads"
	CapabilityOpenAIModels             RequestCapability = "openai_models"
	CapabilityOpenAIFineTuning         RequestCapability = "openai_fine_tuning"
	CapabilityOpenAIBatches            RequestCapability = "openai_batches"
	CapabilityOpenAIVectorStores       RequestCapability = "openai_vector_stores"
	CapabilityOpenAIAssistants         RequestCapability = "openai_assistants"
	CapabilityOpenAIThreads            RequestCapability = "openai_threads"
	CapabilityOpenAIRealtime           RequestCapability = "openai_realtime"
	CapabilityGeminiCompatible         RequestCapability = "gemini_compatible"
	CapabilityGeminiGenerateContent    RequestCapability = "gemini_generate_content"
	CapabilityGeminiStreamGenerate     RequestCapability = "gemini_stream_generate_content"
	CapabilityGeminiCountTokens        RequestCapability = "gemini_count_tokens"
	CapabilityGeminiEmbedContent       RequestCapability = "gemini_embed_content"
	CapabilityGeminiBatchEmbedContents RequestCapability = "gemini_batch_embed_contents"
	CapabilityGeminiModels             RequestCapability = "gemini_models"
	CapabilityGeminiFiles              RequestCapability = "gemini_files"
	CapabilityGeminiUploadFiles        RequestCapability = "gemini_upload_files"
	CapabilityGeminiCachedContents     RequestCapability = "gemini_cached_contents"
	CapabilityGeminiTunedModels        RequestCapability = "gemini_tuned_models"
)

type RequestContext struct {
	ClientType     ClientType
	Family         ProtocolFamily
	Capability     RequestCapability
	UpstreamPath   string
	UnifiedIngress bool
}

type requestContextKey struct{}

type routingScope string

const (
	routingScopeDefault           routingScope = "default"
	routingScopeClaudeCountTokens routingScope = "claude_count_tokens"
	routingScopeOpenAIResponses   routingScope = "openai_responses"
	routingScopeGeminiStream      routingScope = "gemini_stream_generate_content"
)

func detectClipalClientType(path string) (ClientType, bool) {
	requestCtx, ok := detectClipalRequestContext(path)
	if !ok {
		return "", false
	}
	return requestCtx.ClientType, true
}

func detectClipalRequestContext(path string) (RequestContext, bool) {
	path = normalizeUpstreamPath(path)

	switch capability := detectClaudeCapability(path); capability {
	case "":
	default:
		return RequestContext{
			ClientType:     ClientClaude,
			Family:         ProtocolFamilyClaude,
			Capability:     capability,
			UpstreamPath:   path,
			UnifiedIngress: true,
		}, true
	}

	switch capability := detectGeminiCapability(path); capability {
	case "":
	default:
		return RequestContext{
			ClientType:     ClientGemini,
			Family:         ProtocolFamilyGemini,
			Capability:     capability,
			UpstreamPath:   path,
			UnifiedIngress: true,
		}, true
	}

	switch capability := detectOpenAICapability(path); capability {
	case "":
	default:
		return RequestContext{
			ClientType:     ClientOpenAI,
			Family:         ProtocolFamilyOpenAI,
			Capability:     capability,
			UpstreamPath:   path,
			UnifiedIngress: true,
		}, true
	}

	return RequestContext{}, false
}

func requestContextForClientPath(clientType ClientType, path string, unified bool) RequestContext {
	path = normalizeUpstreamPath(path)
	requestCtx := RequestContext{
		ClientType:     clientType,
		UpstreamPath:   path,
		UnifiedIngress: unified,
	}

	switch clientType {
	case ClientClaude:
		requestCtx.Family = ProtocolFamilyClaude
		requestCtx.Capability = capabilityOrDefault(detectClaudeCapability(path), CapabilityClaudeCompatible)
	case ClientOpenAI:
		requestCtx.Family = ProtocolFamilyOpenAI
		requestCtx.Capability = capabilityOrDefault(detectOpenAICapability(path), CapabilityOpenAICompatible)
	case ClientGemini:
		requestCtx.Family = ProtocolFamilyGemini
		requestCtx.Capability = capabilityOrDefault(detectGeminiCapability(path), CapabilityGeminiCompatible)
	}

	return requestCtx
}

func withRequestContext(req *http.Request, requestCtx RequestContext) *http.Request {
	if req == nil {
		return nil
	}
	ctx := context.WithValue(req.Context(), requestContextKey{}, requestCtx)
	return req.WithContext(ctx)
}

func requestContextFromRequest(req *http.Request) (RequestContext, bool) {
	if req == nil {
		return RequestContext{}, false
	}
	requestCtx, ok := req.Context().Value(requestContextKey{}).(RequestContext)
	if !ok {
		return RequestContext{}, false
	}
	return requestCtx, true
}

func routingScopeForRequest(req *http.Request) routingScope {
	requestCtx, ok := requestContextFromRequest(req)
	if !ok {
		return routingScopeDefault
	}
	return routingScopeForCapability(requestCtx.Capability)
}

func routingScopeForCapability(capability RequestCapability) routingScope {
	switch capability {
	case CapabilityClaudeCountTokens:
		return routingScopeClaudeCountTokens
	case CapabilityOpenAIResponses:
		return routingScopeOpenAIResponses
	case CapabilityGeminiStreamGenerate:
		return routingScopeGeminiStream
	default:
		return routingScopeDefault
	}
}

func normalizeUpstreamPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}

func matchesExactPath(path string, want string) bool {
	return path == want || path == want+"/"
}

func capabilityOrDefault(capability RequestCapability, fallback RequestCapability) RequestCapability {
	if capability != "" {
		return capability
	}
	return fallback
}

func isClaudeCompatiblePath(path string) bool {
	return detectClaudeCapability(path) != ""
}

func detectClaudeCapability(path string) RequestCapability {
	switch {
	case isClaudeCountTokensPath(path):
		return CapabilityClaudeCountTokens
	case matchesExactPath(path, "/v1/messages"):
		return CapabilityClaudeMessages
	default:
		return ""
	}
}

func isOpenAICompatiblePath(path string) bool {
	return detectOpenAICapability(path) != ""
}

func detectOpenAICapability(path string) RequestCapability {
	switch {
	case matchesExactPath(path, "/v1/chat/completions"):
		return CapabilityOpenAIChatCompletions
	case matchesExactPath(path, "/v1/completions"):
		return CapabilityOpenAICompletions
	case matchesExactPath(path, "/v1/responses"):
		return CapabilityOpenAIResponses
	case matchesExactPath(path, "/v1/embeddings"):
		return CapabilityOpenAIEmbeddings
	case matchesExactPath(path, "/v1/moderations"):
		return CapabilityOpenAIModerations
	case pathMatchesPrefix(path, "/v1/audio"):
		return CapabilityOpenAIAudio
	case pathMatchesPrefix(path, "/v1/images"):
		return CapabilityOpenAIImages
	case pathMatchesPrefix(path, "/v1/files"):
		return CapabilityOpenAIFiles
	case pathMatchesPrefix(path, "/v1/uploads"):
		return CapabilityOpenAIUploads
	case pathMatchesPrefix(path, "/v1/models"):
		if !isGeminiCompatiblePath(path) {
			return CapabilityOpenAIModels
		}
		return ""
	case pathMatchesPrefix(path, "/v1/fine_tuning"):
		return CapabilityOpenAIFineTuning
	case pathMatchesPrefix(path, "/v1/batches"):
		return CapabilityOpenAIBatches
	case pathMatchesPrefix(path, "/v1/vector_stores"):
		return CapabilityOpenAIVectorStores
	case pathMatchesPrefix(path, "/v1/assistants"):
		return CapabilityOpenAIAssistants
	case pathMatchesPrefix(path, "/v1/threads"):
		return CapabilityOpenAIThreads
	case pathMatchesPrefix(path, "/v1/realtime"):
		return CapabilityOpenAIRealtime
	default:
		return ""
	}
}

func isGeminiCompatiblePath(path string) bool {
	return detectGeminiCapability(path) != ""
}

func detectGeminiCapability(path string) RequestCapability {
	switch {
	case isGeminiMethodPath(path, ":generateContent"):
		return CapabilityGeminiGenerateContent
	case isGeminiMethodPath(path, ":streamGenerateContent"):
		return CapabilityGeminiStreamGenerate
	case isGeminiMethodPath(path, ":countTokens"):
		return CapabilityGeminiCountTokens
	case isGeminiMethodPath(path, ":embedContent"):
		return CapabilityGeminiEmbedContent
	case isGeminiMethodPath(path, ":batchEmbedContents"):
		return CapabilityGeminiBatchEmbedContents
	case isGeminiModelsPath(path):
		return CapabilityGeminiModels
	case isGeminiFilesPath(path):
		return CapabilityGeminiFiles
	case isGeminiUploadFilesPath(path):
		return CapabilityGeminiUploadFiles
	case isGeminiCachedContentsPath(path):
		return CapabilityGeminiCachedContents
	case isGeminiTunedModelsPath(path):
		return CapabilityGeminiTunedModels
	default:
		return ""
	}
}

func isGeminiRPCPath(path string, prefix string) bool {
	if !strings.HasPrefix(path, prefix) {
		return false
	}

	switch {
	case strings.HasSuffix(path, ":generateContent"), strings.HasSuffix(path, ":generateContent/"):
		return true
	case strings.HasSuffix(path, ":streamGenerateContent"), strings.HasSuffix(path, ":streamGenerateContent/"):
		return true
	case strings.HasSuffix(path, ":countTokens"), strings.HasSuffix(path, ":countTokens/"):
		return true
	case strings.HasSuffix(path, ":embedContent"), strings.HasSuffix(path, ":embedContent/"):
		return true
	case strings.HasSuffix(path, ":batchEmbedContents"), strings.HasSuffix(path, ":batchEmbedContents/"):
		return true
	default:
		return false
	}
}

func isGeminiGenerateContentPath(path string) bool {
	return isGeminiMethodPath(path, ":generateContent")
}

func isGeminiStreamGenerateContentPath(path string) bool {
	return isGeminiMethodPath(path, ":streamGenerateContent")
}

func isGeminiMethodPath(path string, method string) bool {
	return isGeminiMethodPathWithPrefix(path, "/v1beta/models/", method) ||
		isGeminiMethodPathWithPrefix(path, "/v1/models/", method)
}

func isGeminiMethodPathWithPrefix(path string, prefix string, method string) bool {
	return strings.HasPrefix(path, prefix) &&
		(strings.HasSuffix(path, method) || strings.HasSuffix(path, method+"/"))
}

func isGeminiModelsPath(path string) bool {
	return pathMatchesPrefix(path, "/v1beta/models") || isGeminiV1ModelMetadataPath(path)
}

func isGeminiFilesPath(path string) bool {
	return pathMatchesPrefix(path, "/v1beta/files")
}

func isGeminiUploadFilesPath(path string) bool {
	return pathMatchesPrefix(path, "/upload/v1beta/files") || pathMatchesPrefix(path, "/upload/v1/files")
}

func isGeminiCachedContentsPath(path string) bool {
	return pathMatchesPrefix(path, "/v1beta/cachedContents") || pathMatchesPrefix(path, "/v1/cachedContents")
}

func isGeminiTunedModelsPath(path string) bool {
	return pathMatchesPrefix(path, "/v1beta/tunedModels") || pathMatchesPrefix(path, "/v1/tunedModels")
}

func isGeminiV1ModelMetadataPath(path string) bool {
	if !pathMatchesPrefix(path, "/v1/models") || matchesExactPath(path, "/v1/models") {
		return false
	}

	trimmed := strings.TrimPrefix(path, "/v1/models/")
	if trimmed == "" {
		return false
	}
	modelID := trimmed
	if idx := strings.Index(modelID, "/"); idx >= 0 {
		modelID = modelID[:idx]
	}
	modelID = strings.TrimSpace(modelID)
	return strings.HasPrefix(modelID, "gemini")
}
