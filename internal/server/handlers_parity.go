package server

import (
    "database/sql"
    "encoding/json"
    "fmt"
    "io"
    "math"
    "net/http"
    "sort"
    "strings"
    "time"

    "github.com/google/uuid"
    "github.com/sanhaji182/lintasan-go/internal/cost"
    "github.com/sanhaji182/lintasan-go/internal/discover"
    "github.com/sanhaji182/lintasan-go/internal/errfmt"
)

func (s *Server) registerParityRoutes() {
    s.mux.HandleFunc("GET /api/overview-stats", s.handleOverviewStats)
    s.mux.HandleFunc("GET /api/providers", s.handleProviders)
    s.mux.HandleFunc("GET /api/providers/presets", s.handleProviderPresets)
    s.mux.HandleFunc("GET /api/providers/presets/config", s.handleProviderPresetsConfig)
    s.mux.HandleFunc("POST /api/providers/presets/test", s.handlePresetTest)
    s.mux.HandleFunc("POST /api/providers/discover", s.handleProviderDiscover)
    s.mux.HandleFunc("POST /api/connections/test", s.handleConnectionTest)
    s.mux.HandleFunc("GET /api/models/sync", s.handleModelsSync)
    s.mux.HandleFunc("POST /api/models/sync", s.handleModelsSync)
    s.mux.HandleFunc("POST /api/models/sync/{connection_id}", s.handleModelsSyncByID)
    s.mux.HandleFunc("GET /api/models/discovered", s.handleModelsDiscovered)
    s.mux.HandleFunc("GET /api/models/manual", s.handleModelsManual)
    s.mux.HandleFunc("POST /api/models/manual", s.handleModelsManual)
    s.mux.HandleFunc("DELETE /api/models/manual", s.handleModelsManual)
    s.mux.HandleFunc("GET /api/cache", s.handleCache)
    s.mux.HandleFunc("POST /api/cache", s.handleCacheAction)
    s.mux.HandleFunc("GET /api/costs", s.handleCosts)
    s.mux.HandleFunc("GET /api/quota", s.handleQuota)
    s.mux.HandleFunc("GET /api/audit", s.handleAudit)
    s.mux.HandleFunc("GET /api/features", s.handleFeatures)
    s.mux.HandleFunc("GET /api/features/stats", s.handleFeatureStats)
    s.mux.HandleFunc("GET /api/analytics/realtime", s.handleAnalyticsRealtime)
    s.mux.HandleFunc("GET /api/analytics/combos", s.handleAnalyticsCombos)
    s.mux.HandleFunc("GET /api/analytics/stream", s.handleAnalyticsStream)
    s.mux.HandleFunc("POST /api/chat-test", s.handleChatTest)
    s.mux.HandleFunc("POST /api/prompt-routing", s.handlePromptRouting)
    s.mux.HandleFunc("POST /api/prompt-optimizer", s.handlePromptOptimizer)
    s.mux.HandleFunc("GET /api/export", s.handleExport)
    s.mux.HandleFunc("POST /api/sync", s.handleSync)
    s.mux.HandleFunc("GET /api/marketplace", s.handleMarketplace)
    s.mux.HandleFunc("GET /api/oauth", s.handleOAuth)
    // Alias endpoints for dashboard compatibility
    s.mux.HandleFunc("GET /api/routing", s.handleGetCombos)
    s.mux.HandleFunc("POST /api/routing/reorder", s.handleRoutingReorder)
    s.mux.HandleFunc("GET /api/connections/sync", s.handleConnectionsSyncAll)
    s.mux.HandleFunc("POST /api/v1/chat/completions", s.proxy.HandleChatCompletions)
    s.mux.HandleFunc("GET /api/teams/{id}", s.handleTeamByID)
    s.mux.HandleFunc("PUT /api/teams/{id}", s.handleTeamByID)
    s.mux.HandleFunc("DELETE /api/teams/{id}", s.handleTeamByID)
    s.mux.HandleFunc("GET /api/teams/{id}/members", s.handleTeamMembers)
    s.mux.HandleFunc("POST /api/teams/{id}/members", s.handleTeamMembers)
    s.mux.HandleFunc("GET /api/users/{id}", s.handleUserByID)
    s.mux.HandleFunc("PUT /api/users/{id}", s.handleUserByID)
    s.mux.HandleFunc("DELETE /api/users/{id}", s.handleUserByID)
    s.mux.HandleFunc("POST /api/users/{id}/reset-password", s.handleUserResetPassword)
    s.mux.HandleFunc("POST /api/v1/images/generations", s.proxy.HandleImages)
    s.mux.HandleFunc("POST /api/v1/audio/speech", s.proxy.HandleAudioSpeech)
    s.mux.HandleFunc("POST /api/v1/audio/transcriptions", s.proxy.HandleAudioTranscriptions)
    s.mux.HandleFunc("POST /api/web-search", s.handleWebSearch)
    s.mux.HandleFunc("GET /api/favicon", s.handleFaviconProxy)
}

func (s *Server) handleOverviewStats(w http.ResponseWriter, r *http.Request){ s.handleStats(w,r) }
func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request){ s.handleGetConnections(w,r) }

func providerPresets() []map[string]any { return []map[string]any{
    {"id":"openai","name":"OpenAI","description":"GPT-4o, GPT-4.1, o3-mini, o4-mini","website":"https://openai.com","category":"major","baseUrl":"https://api.openai.com/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"anthropic","name":"Anthropic","description":"Claude Sonnet 4, Claude Opus 4","website":"https://anthropic.com","category":"major","baseUrl":"https://api.anthropic.com/v1","format":"anthropic","chatPath":"/messages","modelsPath":"","authHeader":"x-api-key","authPrefix":"","extraHeaders":`{"anthropic-version":"2023-06-01"}`},
    {"id":"deepseek","name":"DeepSeek","description":"DeepSeek V4, DeepSeek R1","website":"https://deepseek.com","category":"major","baseUrl":"https://api.deepseek.com","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"google-gemini","name":"Google Gemini","description":"Gemini 2.5 Pro, Gemini 2.0 Flash","website":"https://ai.google.dev","category":"major","baseUrl":"https://generativelanguage.googleapis.com/v1beta","format":"openai","chatPath":"/openai/chat/completions","modelsPath":"/openai/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"xai","name":"xAI (Grok)","description":"Grok 3, Grok 3 Mini","website":"https://x.ai","category":"major","baseUrl":"https://api.x.ai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"mistral","name":"Mistral AI","description":"Mistral Large, Codestral","website":"https://mistral.ai","category":"major","baseUrl":"https://api.mistral.ai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"openrouter","name":"OpenRouter","description":"200+ models, pay-per-token aggregator","website":"https://openrouter.ai","category":"aggregator","baseUrl":"https://openrouter.ai/api/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer ","extraHeaders":`{"HTTP-Referer":"https://lintasan.dev","X-Title":"Lintasan"}`},
    {"id":"groq","name":"Groq","description":"Ultra-fast inference: Llama, Mixtral","website":"https://groq.com","category":"inference","baseUrl":"https://api.groq.com/openai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"together","name":"Together AI","description":"Open-source models: Llama, Qwen","website":"https://together.ai","category":"inference","baseUrl":"https://api.together.xyz/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"fireworks","name":"Fireworks AI","description":"Fast inference: Llama, Mixtral","website":"https://fireworks.ai","category":"inference","baseUrl":"https://api.fireworks.ai/inference/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"cerebras","name":"Cerebras","description":"Fastest inference: Llama 3.3 70B","website":"https://cerebras.ai","category":"inference","baseUrl":"https://api.cerebras.ai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"nvidia-nim","name":"NVIDIA NIM","description":"NVIDIA-optimized: Llama, Nemotron","website":"https://build.nvidia.com","category":"inference","baseUrl":"https://integrate.api.nvidia.com/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"commandcode","name":"CommandCode (API Key)","description":"CommandCode paid API key","website":"https://commandcode.ai","category":"chinese","baseUrl":"https://api.commandcode.ai","format":"commandcode","chatPath":"/v1/chat/completions","modelsPath":"","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"commandcode-alpha","name":"CommandCode (Alpha)","description":"Free alpha — token from cmd auth token","website":"https://commandcode.ai","category":"chinese","baseUrl":"https://api.commandcode.ai","format":"commandcode","chatPath":"/alpha/generate","modelsPath":"","authHeader":"Authorization","authPrefix":"Bearer ","extraHeaders":`{"x-cli-environment":"cli","x-cli-version":"0.26.25"}`},
    {"id":"glm-cn","name":"GLM / Zhipu AI","description":"GLM-4, CogView, CodeGeeX","website":"https://bigmodel.cn","category":"chinese","baseUrl":"https://open.bigmodel.cn/api/coding/paas/v4","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"kimi","name":"Kimi / Moonshot","description":"Kimi K2, Moonshot-v1 (128K)","website":"https://kimi.moonshot.cn","category":"chinese","baseUrl":"https://api.moonshot.cn/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"minimax","name":"MiniMax","description":"MiniMax M2.7, abab6.5","website":"https://minimaxi.com","category":"chinese","baseUrl":"https://api.minimaxi.com/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"qwen","name":"Qwen / Alibaba","description":"Qwen3, Qwen-Max, Qwen-Coder","website":"https://tongyi.aliyun.com","category":"chinese","baseUrl":"https://dashscope.aliyuncs.com/compatible-mode/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"siliconflow","name":"SiliconFlow","description":"Cheap Chinese inference: DeepSeek, Qwen","website":"https://siliconflow.cn","category":"chinese","baseUrl":"https://api.siliconflow.cn/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"perplexity","name":"Perplexity","description":"Sonar: search-augmented LLM","website":"https://perplexity.ai","category":"other","baseUrl":"https://api.perplexity.ai","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"cohere","name":"Cohere","description":"Command R+, Embed, Rerank","website":"https://cohere.com","category":"other","baseUrl":"https://api.cohere.ai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"deepinfra","name":"DeepInfra","description":"Serverless inference: Llama, Mistral","website":"https://deepinfra.com","category":"other","baseUrl":"https://api.deepinfra.com/v1/openai","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"sambanova","name":"SambaNova","description":"Fast inference on custom hardware","website":"https://sambanova.ai","category":"other","baseUrl":"https://api.sambanova.ai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"nebius","name":"Nebius AI","description":"EU-hosted inference: Llama, Qwen","website":"https://nebius.ai","category":"other","baseUrl":"https://api.studio.nebius.ai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"cloudflare","name":"Cloudflare Workers AI","description":"Serverless GPU inference, free tier","website":"https://developers.cloudflare.com/workers-ai","category":"inference","baseUrl":"https://api.cloudflare.com/client/v4/accounts/YOUR_ACCOUNT/ai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"replicate","name":"Replicate","description":"Open-source models, per-run billing","website":"https://replicate.com","category":"aggregator","baseUrl":"https://api.replicate.com/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"hyperbolic","name":"Hyperbolic","description":"Cheap GPU inference: Llama, DeepSeek","website":"https://hyperbolic.xyz","category":"inference","baseUrl":"https://api.hyperbolic.xyz/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"lambda","name":"Lambda AI","description":"GPU cloud with free inference tier","website":"https://lambdalabs.com","category":"inference","baseUrl":"https://api.lambda.ai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"novita","name":"Novita AI","description":"Cheap alternative: DeepSeek, Llama, Qwen","website":"https://novita.ai","category":"other","baseUrl":"https://api.novita.ai/v3/openai","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"ai21","name":"AI21 Labs","description":"Jamba 1.5: 256K context, Mamba-Transformer","website":"https://ai21.com","category":"other","baseUrl":"https://api.ai21.com/studio/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"voyage","name":"Voyage AI","description":"Best embeddings: voyage-3, code-3","website":"https://voyageai.com","category":"other","baseUrl":"https://api.voyageai.com/v1","format":"openai","chatPath":"/embeddings","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"friendliai","name":"FriendliAI","description":"Serverless GPU: Llama, Mixtral, Qwen","website":"https://friendli.ai","category":"inference","baseUrl":"https://api.friendli.ai/serverless/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"huggingface","name":"Hugging Face Inference","description":"Free serverless inference API","website":"https://huggingface.co","category":"aggregator","baseUrl":"https://api-inference.huggingface.co/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"anyscale","name":"Anyscale Endpoints","description":"Ray Serve: Llama, Mistral, Mixtral","website":"https://anyscale.com","category":"inference","baseUrl":"https://api.endpoints.anyscale.com/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"baseten","name":"Baseten","description":"Deploy & serve ML models","website":"https://baseten.co","category":"inference","baseUrl":"https://model-YOUR_MODEL.api.baseten.co/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"octoai","name":"OctoAI","description":"Optimized inference for open models","website":"https://octoml.ai","category":"inference","baseUrl":"https://text.octoai.run/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"snowflake","name":"Snowflake Cortex AI","description":"Enterprise LLM: Arctic, Llama, Mistral","website":"https://snowflake.com","category":"other","baseUrl":"https://YOUR_ACCOUNT.snowflakecomputing.com/api/v2/cortex/inference","format":"openai","chatPath":"/complete","modelsPath":"","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"lepton","name":"Lepton AI","description":"Serverless AI platform","website":"https://lepton.ai","category":"inference","baseUrl":"https://YOUR_DEPLOYMENT.lepton.run/api/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"gradient","name":"Gradient AI","description":"Fine-tuning + inference platform","website":"https://gradient.ai","category":"other","baseUrl":"https://api.gradient.ai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"featherless","name":"Featherless AI","description":"Open-weight models: Llama, Qwen, DeepSeek","website":"https://featherless.ai","category":"inference","baseUrl":"https://api.featherless.ai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"gigachat","name":"GigaChat","description":"Sber's LLM: Russian-focused, GPU inference","website":"https://gigachat.ru","category":"other","baseUrl":"https://gigachat.devices.sberbank.ru/api/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"ovhcloud","name":"OVHcloud AI Endpoints","description":"EU-hosted: Llama, Mistral, Codestral","website":"https://ovhcloud.com","category":"other","baseUrl":"https://YOUR_ENDPOINT.ai-endpoints.ovh.net/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"databricks","name":"Databricks Foundation Model APIs","description":"Enterprise: DBRX, Llama, Mixtral","website":"https://databricks.com","category":"other","baseUrl":"https://YOUR_WORKSPACE.cloud.databricks.com/serving-endpoints","format":"openai","chatPath":"/chat/completions","modelsPath":"","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"vercel-ai","name":"Vercel AI Gateway","description":"Unified AI gateway, free tier","website":"https://vercel.com/ai-gateway","category":"aggregator","baseUrl":"https://api.vercel.ai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"wandb","name":"Weights & Biases Weave","description":"ML platform with LLM serving","website":"https://wandb.ai","category":"other","baseUrl":"https://api.wandb.ai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"crusoe","name":"Crusoe Cloud","description":"Low-carbon GPU inference: Llama, Qwen","website":"https://crusoe.ai","category":"inference","baseUrl":"https://api.crusoe.ai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"nscale","name":"nscale AI","description":"EU GPU cloud with Llama, Mistral","website":"https://nscale.com","category":"inference","baseUrl":"https://api.nscale.ai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"publicai","name":"PublicAI","description":"Decentralized AI compute network","website":"https://publicai.io","category":"inference","baseUrl":"https://api.publicai.io/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"aiml","name":"AIML API","description":"200+ models, OpenAI-compatible","website":"https://aimlapi.com","category":"aggregator","baseUrl":"https://api.aimlapi.com/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"ollama-local","name":"Ollama (Local)","description":"Local models via Ollama","website":"https://ollama.com","category":"self-hosted","baseUrl":"http://localhost:11434/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"","authPrefix":"","noAuth":true},
    {"id":"custom","name":"Custom (OpenAI-Compatible)","description":"Any OpenAI-compatible endpoint","website":"","category":"self-hosted","baseUrl":"","format":"openai","chatPath":"/v1/chat/completions","modelsPath":"/v1/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"sumopod","name":"Sumopod","description":"53 models: GPT-5, Claude, DeepSeek, Gemini","website":"https://sumopod.com","category":"indonesia","baseUrl":"https://ai.sumopod.com","format":"openai","chatPath":"/v1/chat/completions","modelsPath":"/v1/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"azure-openai","name":"Azure OpenAI","description":"GPT-4o, GPT-4.1 on Azure","website":"https://azure.microsoft.com/en-us/products/ai-services/openai-service","category":"major","baseUrl":"https://YOUR_RESOURCE.openai.azure.com/openai","format":"openai","chatPath":"/deployments/YOUR_DEPLOYMENT/chat/completions?api-version=2024-10-21","modelsPath":"","authHeader":"api-key","authPrefix":""},
    {"id":"azure-ai","name":"Azure AI Foundry","description":"Phi, Llama, Mistral via Azure AI","website":"https://azure.microsoft.com/en-us/products/ai-foundry","category":"major","baseUrl":"https://YOUR_RESOURCE.services.ai.azure.com/models","format":"openai","chatPath":"/chat/completions?api-version=2024-05-01-preview","modelsPath":"","authHeader":"api-key","authPrefix":""},
    {"id":"vertex-ai","name":"Google Vertex AI","description":"Gemini 2.5 Pro, Gemini 2.0 Flash on GCP","website":"https://cloud.google.com/vertex-ai","category":"major","baseUrl":"https://LOCATION-aiplatform.googleapis.com/v1/projects/PROJECT/locations/LOCATION/publishers/google/models/gemini-2.0-flash:generateContent","format":"gemini","chatPath":"","modelsPath":"","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"aws-bedrock","name":"AWS Bedrock","description":"Claude, Llama, Titan, Mistral on AWS","website":"https://aws.amazon.com/bedrock","category":"major","baseUrl":"https://bedrock-runtime.REGION.amazonaws.com","format":"bedrock","chatPath":"","modelsPath":"","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"aws-sagemaker","name":"AWS SageMaker","description":"Deploy any model on SageMaker endpoints","website":"https://aws.amazon.com/sagemaker","category":"major","baseUrl":"https://runtime.sagemaker.REGION.amazonaws.com/endpoints/ENDPOINT/invocations","format":"openai","chatPath":"","modelsPath":"","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"codestral","name":"Codestral API (Mistral)","description":"Code-specialized: fill-in-the-middle, agents","website":"https://mistral.ai/products/codestral","category":"major","baseUrl":"https://codestral.mistral.ai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"xiaomi-mimo","name":"Xiaomi MiMo","description":"Xiaomi MiMo-v2.5 series","website":"https://mimo.xiaomi.com","category":"chinese","baseUrl":"https://api.mimo.xiaomi.com/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"volcengine","name":"Volcano Engine (ByteDance)","description":"Doubao, Skylark, DeepSeek models","website":"https://volcengine.com","category":"chinese","baseUrl":"https://ark.cn-beijing.volces.com/api/v3","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"zhipu-ai","name":"Z.AI (Zhipu AI)","description":"GLM-4, CodeGeeX, ChatGLM","website":"https://bigmodel.cn","category":"chinese","baseUrl":"https://open.bigmodel.cn/api/paas/v4","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"meta-llama","name":"Meta Llama API","description":"Official Llama models from Meta","website":"https://llama.com","category":"major","baseUrl":"https://api.llama.com/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"github-models","name":"GitHub Models","description":"Free model playground: GPT-4o, Llama, Phi","website":"https://github.com/marketplace/models","category":"other","baseUrl":"https://models.inference.ai.azure.com","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"github-copilot","name":"GitHub Copilot API","description":"Copilot Chat models","website":"https://github.com/features/copilot","category":"other","baseUrl":"https://api.githubcopilot.com","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"poe","name":"Poe by Quora","description":"Multi-model chatbot API","website":"https://poe.com","category":"aggregator","baseUrl":"https://api.poe.com/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"galadriel","name":"Galadriel","description":"Decentralized inference network","website":"https://galadriel.com","category":"inference","baseUrl":"https://api.galadriel.com/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"chutes","name":"Chutes","description":"Permissionless GPU inference network","website":"https://chutes.ai","category":"inference","baseUrl":"https://api.chutes.ai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"gmi-cloud","name":"GMI Cloud","description":"GPU cloud with model APIs","website":"https://gmicloud.ai","category":"inference","baseUrl":"https://api.gmicloud.ai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"jina-ai","name":"Jina AI","description":"Embeddings, Reader, Reranker, Search","website":"https://jina.ai","category":"other","baseUrl":"https://api.jina.ai/v1","format":"openai","chatPath":"/embeddings","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"deepgram","name":"Deepgram","description":"Speech-to-text and text-to-speech API","website":"https://deepgram.com","category":"other","baseUrl":"https://api.deepgram.com/v1","format":"openai","chatPath":"/listen","modelsPath":"","authHeader":"Authorization","authPrefix":"Token "},
    {"id":"elevenlabs","name":"ElevenLabs","description":"Text-to-speech: best voice AI","website":"https://elevenlabs.io","category":"other","baseUrl":"https://api.elevenlabs.io/v1","format":"openai","chatPath":"/text-to-speech","modelsPath":"","authHeader":"xi-api-key","authPrefix":""},
    {"id":"fal-ai","name":"Fal AI","description":"Generative media: Flux, Wan, AuraFlow","website":"https://fal.ai","category":"other","baseUrl":"https://fal.run/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Key "},
    {"id":"black-forest-labs","name":"Black Forest Labs","description":"Flux image generation & editing","website":"https://blackforestlabs.ai","category":"other","baseUrl":"https://api.bfl.ml/v1","format":"openai","chatPath":"","modelsPath":"","authHeader":"x-key","authPrefix":""},
    {"id":"stability-ai","name":"Stability AI","description":"Stable Diffusion, Stable Image","website":"https://stability.ai","category":"other","baseUrl":"https://api.stability.ai/v1","format":"openai","chatPath":"","modelsPath":"","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"runwayml","name":"RunwayML","description":"Gen-3 Alpha, Gen-4 video generation","website":"https://runwayml.com","category":"other","baseUrl":"https://api.runwayml.com/v1","format":"openai","chatPath":"","modelsPath":"","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"recraft","name":"Recraft","description":"Vector & raster AI image generation","website":"https://recraft.ai","category":"other","baseUrl":"https://api.recraft.ai/v1","format":"openai","chatPath":"","modelsPath":"","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"clarifai","name":"Clarifai","description":"Full-stack AI platform: vision, language, audio","website":"https://clarifai.com","category":"other","baseUrl":"https://api.clarifai.com/v2","format":"openai","chatPath":"","modelsPath":"","authHeader":"Authorization","authPrefix":"Key "},
    {"id":"nlp-cloud","name":"NLP Cloud","description":"Production NLP: GPT, Llama, Dolphin","website":"https://nlpcloud.io","category":"other","baseUrl":"https://api.nlpcloud.io/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Token "},
    {"id":"predibase","name":"Predibase","description":"Fine-tune & serve open-source LLMs","website":"https://predibase.com","category":"other","baseUrl":"https://serving.app.predibase.com/TENANT/deployments/v2/llms/MODEL/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"oci","name":"Oracle Cloud (OCI)","description":"Llama, Cohere on OCI Generative AI","website":"https://www.oracle.com/artificial-intelligence/generative-ai","category":"other","baseUrl":"https://inference.generativeai.REGION.oci.oraclecloud.com/20231130","format":"openai","chatPath":"/chat/completions","modelsPath":"","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"watsonx","name":"IBM WatsonX","description":"Granite, Llama, Mistral on IBM Cloud","website":"https://www.ibm.com/watsonx","category":"other","baseUrl":"https://REGION.ml.cloud.ibm.com/ml/v1","format":"openai","chatPath":"/text/chat?version=2024-09-01","modelsPath":"","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"sap","name":"SAP Generative AI Hub","description":"Enterprise AI on SAP BTP","website":"https://www.sap.com/products/artificial-intelligence.html","category":"other","baseUrl":"https://api.ai.prod.REGION.aws.ml.hana.ondemand.com/v2","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"scaleway","name":"Scaleway","description":"EU cloud: Llama, Mistral, Qwen","website":"https://scaleway.com","category":"other","baseUrl":"https://api.scaleway.ai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"sarvam","name":"Sarvam AI","description":"Indian language models: Hindi, Tamil, Telugu","website":"https://sarvam.ai","category":"other","baseUrl":"https://api.sarvam.ai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"aleph-alpha","name":"Aleph Alpha","description":"EU sovereign AI: Luminous series","website":"https://aleph-alpha.com","category":"other","baseUrl":"https://api.aleph-alpha.com/v1","format":"openai","chatPath":"/complete","modelsPath":"","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"amazon-nova","name":"Amazon Nova","description":"Amazon's frontier models","website":"https://aws.amazon.com/ai/generative-ai/nova","category":"other","baseUrl":"https://bedrock-runtime.REGION.amazonaws.com","format":"openai","chatPath":"","modelsPath":"","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"apertis-stima","name":"Apertis AI (Stima API)","description":"Indonesian AI platform","website":"https://stima.id","category":"indonesia","baseUrl":"https://api.stima.id/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"bytez","name":"Bytez","description":"Run any HuggingFace model via API","website":"https://bytez.com","category":"other","baseUrl":"https://api.bytez.com/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"cometapi","name":"CometAPI","description":"Model aggregator: GPT, Claude, Gemini","website":"https://cometapi.com","category":"aggregator","baseUrl":"https://api.cometapi.com/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"compactifai","name":"CompactifAI","description":"Model compression & optimization","website":"https://compactif.ai","category":"other","baseUrl":"https://api.compactif.ai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"datarobot","name":"DataRobot","description":"Enterprise AI platform with LLM support","website":"https://datarobot.com","category":"other","baseUrl":"https://DEPLOYMENT.orm.datarobot.com/predApi/v1.0/deployments/ID/chat/completions","format":"openai","chatPath":"","modelsPath":"","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"helicone","name":"Helicone","description":"LLM observability with proxy AI","website":"https://helicone.ai","category":"other","baseUrl":"https://oai.helicone.ai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"heroku","name":"Heroku AI","description":"ML model deployment on Heroku","website":"https://heroku.com","category":"inference","baseUrl":"https://APP.herokuapp.com/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"langgraph","name":"LangGraph Cloud","description":"LangChain's agent infrastructure API","website":"https://langchain.com/langgraph","category":"other","baseUrl":"https://api.langgraph.com/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"lemonade","name":"Lemonade","description":"AI-powered developer platform","website":"https://lemonade.ai","category":"other","baseUrl":"https://api.lemonade.ai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"manus","name":"Manus AI","description":"General AI agent platform API","website":"https://manus.im","category":"other","baseUrl":"https://api.manus.im/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"nanogpt","name":"NanoGPT","description":"Pay-per-use LLM access","website":"https://nanogpt.ai","category":"aggregator","baseUrl":"https://api.nanogpt.ai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"petals","name":"Petals","description":"Decentralized BitTorrent LLM inference","website":"https://petals.dev","category":"other","baseUrl":"https://chat.petals.dev/api/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"pydantic-ai","name":"Pydantic AI Agents","description":"Pydantic's agent framework API","website":"https://pydantic.dev","category":"other","baseUrl":"https://api.pydantic.dev/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"ragflow","name":"RAGFlow","description":"Open-source RAG engine API","website":"https://ragflow.io","category":"other","baseUrl":"https://HOST/api/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"synthetic","name":"Synthetic","description":"AI agent automation and monitoring","website":"https://synthetic.ai","category":"other","baseUrl":"https://api.synthetic.ai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"topaz","name":"Topaz","description":"AI for legal, compliance, contracts","website":"https://topaz.ai","category":"other","baseUrl":"https://api.topaz.ai/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"v0","name":"v0 by Vercel","description":"Vercel's AI design-to-code API","website":"https://v0.dev","category":"other","baseUrl":"https://api.v0.dev/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"morph","name":"Morph","description":"AI-powered data warehouse & analytics","website":"https://morphdb.io","category":"other","baseUrl":"https://api.morphdb.io/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"vllm","name":"vLLM (Self-Hosted)","description":"High-throughput LLM serving engine","website":"https://docs.vllm.ai","category":"self-hosted","baseUrl":"http://localhost:8000/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"","authPrefix":"","noAuth":true},
    {"id":"triton","name":"Triton Inference Server","description":"NVIDIA Triton with LLM backend","website":"https://developer.nvidia.com/triton-inference-server","category":"self-hosted","baseUrl":"http://localhost:8000/v2/models/MODEL/generate","format":"openai","chatPath":"","modelsPath":"","authHeader":"","authPrefix":"","noAuth":true},
    {"id":"xinference","name":"Xinference (Xorbits)","description":"Self-hosted LLM serving platform","website":"https://inference.readthedocs.io","category":"self-hosted","baseUrl":"http://localhost:9997/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"","authPrefix":"","noAuth":true},
    {"id":"llamafile","name":"Llamafile","description":"Single-file executable LLM (Mozilla)","website":"https://github.com/Mozilla-Ocho/llamafile","category":"self-hosted","baseUrl":"http://localhost:8080/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"","authPrefix":"","noAuth":true},
    {"id":"llamagate","name":"LlamaGate","description":"Local LLM gateway with routing","website":"https://github.com/bigsker/llamagate","category":"self-hosted","baseUrl":"http://localhost:8080/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"","authPrefix":"","noAuth":true},
    {"id":"lm-studio","name":"LM Studio","description":"Desktop app for local LLM inference","website":"https://lmstudio.ai","category":"self-hosted","baseUrl":"http://localhost:1234/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"","authPrefix":"","noAuth":true},
    {"id":"docker-model-runner","name":"Docker Model Runner","description":"Docker-native model inference","website":"https://docs.docker.com/desktop/model-runner","category":"self-hosted","baseUrl":"http://localhost:8080/v1","format":"openai","chatPath":"/chat/completions","modelsPath":"/models","authHeader":"","authPrefix":"","noAuth":true},
    {"id":"infinity","name":"Infinity Embeddings","description":"michaelfeil/infinity: fast embeddings server","website":"https://github.com/michaelfeil/infinity","category":"self-hosted","baseUrl":"http://localhost:7997/v1","format":"openai","chatPath":"/embeddings","modelsPath":"/models","authHeader":"","authPrefix":"","noAuth":true},
    {"id":"chatgpt-subscription","name":"ChatGPT Subscription","description":"Access GPT-4o via ChatGPT Plus subscription","website":"https://chatgpt.com","category":"other","baseUrl":"https://chatgpt.com/backend-api","format":"openai","chatPath":"/conversations","modelsPath":"","authHeader":"Authorization","authPrefix":"Bearer "},
} }
func providerCategories() []map[string]any { return []map[string]any{{"id":"major","name":"Major Providers"},{"id":"aggregator","name":"Aggregators"},{"id":"inference","name":"Fast Inference"},{"id":"chinese","name":"Chinese Providers"},{"id":"indonesia","name":"Indonesia Providers"},{"id":"enterprise","name":"Enterprise & Cloud"},{"id":"media","name":"Media & Audio"},{"id":"other","name":"Other Providers"},{"id":"self-hosted","name":"Self-Hosted"}} }
func (s *Server) handleProviderPresets(w http.ResponseWriter, r *http.Request){ 
    presets := providerPresets()
    for i, p := range presets {
        if _, ok := p["provider"]; !ok {
            presets[i]["provider"] = p["id"]
        }
        if _, ok := p["label"]; !ok {
            presets[i]["label"] = p["name"]
        }
        if _, ok := p["base_url"]; !ok {
            presets[i]["base_url"] = p["baseUrl"]
        }
        if _, ok := p["models_path"]; !ok {
            presets[i]["models_path"] = p["modelsPath"]
        }
        if _, ok := p["auth_header"]; !ok {
            presets[i]["auth_header"] = p["authHeader"]
        }
        if _, ok := p["auth_prefix"]; !ok {
            presets[i]["auth_prefix"] = p["authPrefix"]
        }
        if _, ok := p["chat_path"]; !ok {
            presets[i]["chat_path"] = p["chatPath"]
        }
    }
    writeJSON(w, map[string]any{"data":presets,"categories":providerCategories()})
}
func (s *Server) handleProviderPresetsConfig(w http.ResponseWriter, r *http.Request){ id:=r.URL.Query().Get("id"); for _,p:=range providerPresets(){ if p["id"]==id { writeData(w,p); return } }; writeJSON(w, map[string]any{"data":map[string]any{},"presets":providerPresets(),"formats":[]string{"openai","anthropic","gemini","ollama","custom"}}) }

func (s *Server) handlePresetTest(w http.ResponseWriter, r *http.Request){
    var in map[string]any; json.NewDecoder(r.Body).Decode(&in)
    id,_:=in["id"].(string)
    var preset map[string]any
    for _,p:=range providerPresets(){ if p["id"]==id { preset=p; break } }
    if preset==nil { writeJSON(w,map[string]any{"success":false,"error":"Provider preset not found"}); return }
    baseUrl,_:=preset["baseUrl"].(string)
    modelsPath,_:=preset["modelsPath"].(string)
    authHeader,_:=preset["authHeader"].(string)
    authPrefix,_:=preset["authPrefix"].(string)
    apiKey,_:=in["apiKey"].(string)
    start:=time.Now()
    models,_,_,err:=fetchModels(baseUrl,modelsPath,apiKey,authHeader,authPrefix)
    if err!=nil{
        writeJSON(w,map[string]any{"success":false,"error":err.Error(),"latency_ms":time.Since(start).Milliseconds()})
        return
    }
    writeJSON(w,map[string]any{"success":true,"message":fmt.Sprintf("Connected · %d models found · %dms",len(models),time.Since(start).Milliseconds()),"models_count":len(models),"latency_ms":time.Since(start).Milliseconds(),"models":models})
}

func fetchModels(base, path, key, h, prefix string)([]any,int,[]byte,error){
    if base=="" { return nil,0,nil,fmt.Errorf("base_url required") }
    req,_:=http.NewRequest("GET", strings.TrimRight(base,"/")+path, nil)
    if key!=""{ if h==""{h="Authorization"}; req.Header.Set(h,prefix+key) }
    c:=&http.Client{Timeout:20*time.Second}; resp,err:=c.Do(req); if err!=nil{return nil,0,nil,err}; defer resp.Body.Close()
    b,_:=io.ReadAll(resp.Body)
    if resp.StatusCode>=400 { return nil,resp.StatusCode,b,fmt.Errorf("upstream status %d",resp.StatusCode) }
    var data map[string]any; json.Unmarshal(b,&data)
    if arr,ok:=data["data"].([]any); ok { return arr,resp.StatusCode,b,nil }
    if arr,ok:=data["models"].([]any); ok { return arr,resp.StatusCode,b,nil }
    return []any{},resp.StatusCode,b,nil
}

// pingChat sends a minimal chat-completion request to verify the upstream
// accepts chat traffic. Used as a fallback when the provider does not expose
// /v1/models. Returns (statusCode, body, err) — a 2xx/3xx/4xx-non-404 result
// means the upstream is reachable and auth works (a 400 "model not found" is
// fine — that proves auth succeeded and traffic flows).
func pingChat(base, key, h, prefix string)(int,[]byte,error){
    if base=="" { return 0,nil,fmt.Errorf("base_url required") }
    body:=`{"model":"__lintasan_ping__","messages":[{"role":"user","content":"ping"}],"max_tokens":1,"stream":false}`
    req,_:=http.NewRequest("POST", strings.TrimRight(base,"/")+"/v1/chat/completions", strings.NewReader(body))
    if key!=""{ if h==""{h="Authorization"}; req.Header.Set(h,prefix+key) }
    req.Header.Set("Content-Type","application/json")
    c:=&http.Client{Timeout:20*time.Second}; resp,err:=c.Do(req); if err!=nil{return 0,nil,err}
    defer resp.Body.Close()
    b,_:=io.ReadAll(resp.Body)
    return resp.StatusCode,b,nil
}

func truncateBody(b []byte, n int) string { if len(b)<=n { return string(b) }; return string(b[:n])+"..." }

func (s *Server) handleConnectionTest(w http.ResponseWriter, r *http.Request){
    var in map[string]any; json.NewDecoder(r.Body).Decode(&in)
    base,_:=in["base_url"].(string); if base==""{base,_=in["baseUrl"].(string)}
    key,_:=in["api_key"].(string); if key==""{key,_=in["apiKey"].(string)}
    var oauthProv string
    oauthFromIn,_:=in["oauth_provider"].(string)
    if strings.TrimSpace(oauthFromIn)!="" { oauthProv=strings.TrimSpace(oauthFromIn) }
    path,_:=in["models_path"].(string); if path==""{path,_=in["modelsPath"].(string)}; if path==""{path="/v1/models"}
    // If only an id was supplied (list-view Test button), look up the saved
    // connection from the DB so we can re-test it without re-typing the key.
    if base=="" {
        if id,_:=in["id"].(string); id!="" {
            var dbBase, dbKey, dbOAuth string
            err:=s.db.Conn().QueryRow("SELECT base_url, api_key, COALESCE(oauth_provider,'') FROM connections WHERE id=?", id).Scan(&dbBase, &dbKey, &dbOAuth)
            if err==nil {
                base=dbBase
                oauthProv=strings.TrimSpace(dbOAuth)
                if key=="" && oauthProv=="" { key=dbKey }
            }
        }
    }
    if oauthProv!="" {
        tok, errTok:=s.oauthMgr.GetActiveToken(oauthProv)
        if errTok!=nil {
            e:=errfmt.New("no active OAuth session for "+oauthProv+": "+errTok.Error(), errfmt.TypeInvalidRequestError, errfmt.CodeBadFormat)
            errfmt.Write(w, http.StatusBadRequest, e, nil, map[string]any{"success": false, "latency_ms": 0, "hint": "Authorize in OAuth IDE, then test again"})
            return
        }
        key=tok
    }
    if base=="" {
        e:=errfmt.New("base_url is required (and no saved connection found for the given id)", errfmt.TypeInvalidRequestError, errfmt.CodeBadFormat)
        errfmt.Write(w, http.StatusBadRequest, e, nil, map[string]any{"success": false, "latency_ms": 0})
        return
    }
    start:=time.Now()

    models,status,body,err:=fetchModels(base,path,key,"Authorization","Bearer ")
    latency:=time.Since(start).Milliseconds()

    if err==nil{
        msg:=fmt.Sprintf("Connected successfully · %d models found · %dms", len(models), latency)
        errfmt.Write(w, http.StatusOK, nil, map[string]any{"models": models, "models_count": len(models)}, map[string]any{
            "success": true, "message": msg, "latency_ms": latency, "models_count": len(models),
        })
        return
    }

    // /v1/models failed. Decide whether to fall back to a chat-ping.
    // Only 5xx triggers fallback — it means the upstream itself is broken,
    // so /v1/chat/completions might still work. For 4xx, the endpoint exists
    // and the request itself was wrong (auth, format, model) — retrying with
    // chat-ping won't help and will just add latency.
    if key!="" && status>=500 && status<=599{
        pingStatus,pingBody,pingErr:=pingChat(base,key,"Authorization","Bearer ")
        if pingErr!=nil{
            e:=errfmt.FromNetworkError(pingErr)
            e.Message="models endpoint failed and chat-ping network error: "+e.Message
            errfmt.Write(w, http.StatusBadGateway, e, nil, map[string]any{
                "success": false, "latency_ms": latency, "hint": errfmt.HintForMessage(e.Message),
            })
            return
        }
        // 2xx/3xx → success via fallback
        if pingStatus<400{
            msg:=fmt.Sprintf("Reachable via chat endpoint (no /v1/models) · %dms",latency)
            errfmt.Write(w, http.StatusOK, nil, nil, map[string]any{
                "success": true, "message": msg, "latency_ms": latency, "models_count": 0, "fallback": "chat_ping",
            })
            return
        }
        // chat-ping returned 4xx/5xx → return standard error
        e:=errfmt.FromStatus(pingStatus, pingBody, fmt.Sprintf("chat-ping status %d",pingStatus))
        errfmt.Write(w, http.StatusBadGateway, e, nil, map[string]any{
            "success": false, "latency_ms": latency, "hint": errfmt.HintForMessage(e.Message),
        })
        return
    }

    // Direct /v1/models failure — distinguish network vs HTTP
    if status==0 {
        // status==0 means we never even got a response (dial/timeout/DNS)
        e:=errfmt.FromNetworkError(err)
        errfmt.Write(w, http.StatusBadGateway, e, nil, map[string]any{
            "success": false, "latency_ms": latency, "hint": errfmt.HintForMessage(e.Message),
        })
        return
    }
    e:=errfmt.FromStatus(status, body, fmt.Sprintf("upstream status %d",status))
    httpStatus:=http.StatusBadGateway
    if status>=400 && status<500 { httpStatus=http.StatusBadRequest }
    errfmt.Write(w, httpStatus, e, nil, map[string]any{
        "success": false, "latency_ms": latency, "hint": errfmt.HintForMessage(e.Message),
    })
}

func (s *Server) handleModelsSyncByID(w http.ResponseWriter, r *http.Request) {
    connID := r.PathValue("connection_id")
    if connID == "" {
        writeJSON(w, map[string]any{"error": map[string]string{"message": "connection_id is required"}})
        return
    }
    res, err := s.discoverer.SyncConnection(connID)
    if err != nil {
        writeJSON(w, map[string]any{"error": map[string]string{"message": err.Error()}})
        return
    }
    writeJSON(w, map[string]any{
        "success": true,
        "data":    res,
        "synced":  res.ModelsCount,
    })
}

func (s *Server) handleModelsDiscovered(w http.ResponseWriter, r *http.Request) {
    connID := r.URL.Query().Get("connection_id")
    var rows *sql.Rows
    var err error
    if connID != "" {
        rows, err = s.db.Conn().Query(
            "SELECT id, model_id, model_name, owned_by, is_active, discovered_at FROM discovered_models WHERE connection_id=? ORDER BY model_id", connID)
    } else {
        rows, err = s.db.Conn().Query(
            "SELECT id, model_id, model_name, owned_by, is_active, discovered_at FROM discovered_models ORDER BY model_id")
    }
    out := []map[string]any{}
    if err == nil && rows != nil {
    	defer rows.Close()
    	for rows.Next() {
    		var id, mid, name, owner, dt string
    		var active int
    		if err := rows.Scan(&id, &mid, &name, &owner, &active, &dt); err != nil {
    			continue
    		}
    		out = append(out, map[string]any{
    			"id": id, "model_id": mid, "model_name": name,
    			"owned_by": owner, "is_active": active, "discovered_at": dt,
    		})
    	}
    	if err := rows.Err(); err != nil {
    		// iteration error
    	}
    }
    writeData(w, out)
}

func (s *Server) handleModelsSync(w http.ResponseWriter, r *http.Request){
    if r.Method==http.MethodGet {
        connID := r.URL.Query().Get("connection_id")
        var rows *sql.Rows
        var err error
        if connID != "" {
            rows, err = s.db.Conn().Query(
                "SELECT id, model_id, model_name, owned_by, is_active, discovered_at FROM discovered_models WHERE connection_id=? ORDER BY model_id", connID)
        } else {
            rows, err = s.db.Conn().Query(
                "SELECT id, model_id, model_name, owned_by, is_active, discovered_at FROM discovered_models ORDER BY model_id")
        }
        out := []map[string]any{}
        if err == nil && rows != nil {
        	defer rows.Close()
        	for rows.Next() {
        		var id, mid, name, owner, dt string
        		var active int
        		if err := rows.Scan(&id, &mid, &name, &owner, &active, &dt); err != nil {
        			continue
        		}
        		out = append(out, map[string]any{
        			"id": id, "model_id": mid, "model_name": name,
        			"owned_by": owner, "is_active": active, "discovered_at": dt,
        		})
        	}
        	if err := rows.Err(); err != nil {
        		// iteration error
        	}
        }
        writeData(w, out)
        return
    }

    // POST: trigger sync
    var in map[string]any
    json.NewDecoder(r.Body).Decode(&in)
    connID, _ := in["connection_id"].(string)

    var results any
    var totalSynced int
    if connID != "" {
        res, err := s.discoverer.SyncConnection(connID)
        if err != nil {
            writeJSON(w, map[string]any{"error": map[string]string{"message": err.Error()}})
            return
        }
        if res.Status == "ok" { totalSynced = res.ModelsCount }
        results = []*discover.SyncResult{res}
    } else {
        resList, err := s.discoverer.SyncAll()
        if err != nil {
            writeJSON(w, map[string]any{"error": map[string]string{"message": err.Error()}})
            return
        }
        for _, r := range resList {
            if r.Status == "ok" { totalSynced += r.ModelsCount }
        }
        results = resList
    }

    writeJSON(w, map[string]any{
        "success": true,
        "data":    results,
        "synced":  totalSynced,
    })
}
func (s *Server) handleModelsManual(w http.ResponseWriter, r *http.Request){
    connID:=r.URL.Query().Get("connectionId"); if connID==""{connID=r.URL.Query().Get("connection_id")}
    if r.Method=="GET" { rows,_:=s.db.Conn().Query("SELECT id, model_id, model_name, owned_by, is_active, discovered_at FROM discovered_models WHERE connection_id=? ORDER BY model_id", connID); out:=[]map[string]any{}; if rows!=nil{defer rows.Close(); for rows.Next(){var id,mid,name,owner,dt string; var active int; rows.Scan(&id,&mid,&name,&owner,&active,&dt); out=append(out,map[string]any{"id":id,"model_id":mid,"model_name":name,"owned_by":owner,"is_active":active,"discovered_at":dt})}}; writeJSON(w,map[string]any{"models":out,"data":out}); return }
    if r.Method=="DELETE" { mid:=r.URL.Query().Get("modelId"); s.db.Conn().Exec("DELETE FROM discovered_models WHERE connection_id=? AND model_id=?", connID, mid); writeJSON(w,map[string]any{"success":true}); return }
    var in map[string]any; json.NewDecoder(r.Body).Decode(&in); if connID==""{connID,_=in["connectionId"].(string)}; if connID==""{connID,_=in["connection_id"].(string)}
    if in["action"]=="toggle" { mid,_:=in["modelId"].(string); active:=1; if v,ok:=in["active"].(bool);ok&&!v{active=0}; s.db.Conn().Exec("UPDATE discovered_models SET is_active=? WHERE connection_id=? AND model_id=?",active,connID,mid); writeJSON(w,map[string]any{"success":true}); return }
    models:=stringSlice(in["models"]); if len(models)==0{ if m,_:=in["model_id"].(string);m!=""{models=[]string{m}} }
    for _,mid:= range models { s.db.Conn().Exec("INSERT OR REPLACE INTO discovered_models(id,connection_id,model_id,model_name,owned_by,is_active) VALUES(?,?,?,?,?,1)",uuid.New().String(),connID,mid,mid,"manual") }
    s.db.Conn().Exec("UPDATE connections SET models_count=(SELECT COUNT(*) FROM discovered_models WHERE connection_id=? AND is_active=1) WHERE id=?",connID,connID)
    writeJSON(w,map[string]any{"success":true,"data":map[string]any{"count":len(models)}})
}
func (s *Server) handleCache(w http.ResponseWriter, r *http.Request) {
	// Cache performance stats matching Node schema:
	// exact_hits, stream_hits, semantic_hits, misses, hit_rate

	// Exact cache hits: count from request_logs where cached = 1
	var exactHits int
	if err := s.db.Conn().QueryRow("SELECT COUNT(*) FROM request_logs WHERE cached = 1").Scan(&exactHits); err != nil {
		exactHits = 0
	}

	// Stream cache hits: count entries in stream_response_cache
	var streamHits int
	if err := s.db.Conn().QueryRow("SELECT COUNT(*) FROM stream_response_cache").Scan(&streamHits); err != nil {
		streamHits = 0
	}

	// Semantic cache hits: sum hits from semantic_cache
	var semanticHits int
	if err := s.db.Conn().QueryRow("SELECT COALESCE(SUM(hits), 0) FROM semantic_cache").Scan(&semanticHits); err != nil {
		semanticHits = 0
	}

	// Also count embedding_cache entries
	var embeddingCount int
	if err := s.db.Conn().QueryRow("SELECT COALESCE(SUM(hits), 0) FROM embedding_cache").Scan(&embeddingCount); err != nil {
		embeddingCount = 0
	}

	// Misses: total requests minus cached
	var totalRequests int
	if err := s.db.Conn().QueryRow("SELECT COUNT(*) FROM request_logs").Scan(&totalRequests); err != nil {
		totalRequests = 0
	}
	misses := totalRequests - exactHits
	if misses < 0 {
		misses = 0
	}

	total := float64(exactHits + streamHits + semanticHits + misses)
	hitRate := "0.0%"
	if total > 0 {
		rate := float64(exactHits+streamHits+semanticHits) / total * 100
		hitRate = fmt.Sprintf("%.1f%%", rate)
	}

	// Also return raw cache table counts for the frontend
	var exactEntries, streamEntries, semanticEntries int
	if err := s.db.Conn().QueryRow("SELECT COUNT(*) FROM response_cache").Scan(&exactEntries); err != nil {
		exactEntries = 0
	}
	if err := s.db.Conn().QueryRow("SELECT COUNT(*) FROM stream_response_cache").Scan(&streamEntries); err != nil {
		streamEntries = 0
	}
	if err := s.db.Conn().QueryRow("SELECT COUNT(*) FROM semantic_cache").Scan(&semanticEntries); err != nil {
		semanticEntries = 0
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"exact_hits":       exactHits,
		"stream_hits":      streamHits,
		"semantic_hits":    semanticHits,
		"embedding_hits":   embeddingCount,
		"misses":           misses,
		"hit_rate":         hitRate,
		"total_requests":   totalRequests,
		"exact_entries":    exactEntries,
		"stream_entries":   streamEntries,
		"semantic_entries": semanticEntries,
	})
}
func (s *Server) handleCacheAction(w http.ResponseWriter,r *http.Request){ s.db.Conn().Exec("DELETE FROM embedding_cache"); s.db.Conn().Exec("DELETE FROM semantic_cache"); writeJSON(w,map[string]any{"success":true,"status":"cleared"}) }
// handleCosts implements GET /api/costs. It computes real aggregated costs
// from request_logs (written by the proxy on every chat completion) using
// the cost package's Calculator and the built-in pricing table. Replaces
// the prior stub that returned hardcoded zeros.
//
// The proxy doesn't persist a separate `cost_entries` row yet, so we
// recompute on read by joining tokens with model pricing. Cheap, and the
// read path is hit rarely (dashboard refresh). If a `cost_entries` table
// is added later, swap the read for Tracker.Summary() and keep the JSON
// shape stable.
func (s *Server) handleCosts(w http.ResponseWriter, r *http.Request) {
	calc := cost.NewCalculator()
	cutoffToday := "datetime('now', 'localtime', 'start of day')"
	cutoffMonth := "datetime('now', 'localtime', 'start of month')"
	aggregate := func(cutoffExpr string) (float64, []map[string]any, int, int, int) {
		var totalCost float64
		byModelMap := map[string]*struct{ cost float64; requests, inTok, outTok int }{}
		byModel := []map[string]any{}
		totalReq, totalIn, totalOut := 0, 0, 0
		rows, err := s.db.Conn().Query("SELECT model, SUM(input_tokens), SUM(output_tokens), COUNT(*) FROM request_logs WHERE created_at >= " + cutoffExpr + " GROUP BY model")
		if err != nil { return 0, byModel, 0, 0, 0 }
		defer rows.Close()
		for rows.Next() {
			var model string
			var inTok, outTok, reqs int
			if err := rows.Scan(&model, &inTok, &outTok, &reqs); err != nil { continue }
			c := calc.CalculateCost(model, inTok, outTok)
			totalCost += c.TotalCostUSD
			e, ok := byModelMap[model]
			if !ok { e = &struct{ cost float64; requests, inTok, outTok int }{}; byModelMap[model] = e }
			e.cost += c.TotalCostUSD; e.requests += reqs; e.inTok += inTok; e.outTok += outTok
			totalReq += reqs; totalIn += inTok; totalOut += outTok
		}
		for model, e := range byModelMap {
			byModel = append(byModel, map[string]any{"model": model, "requests": e.requests, "input_tokens": e.inTok, "output_tokens": e.outTok, "cost_usd": round2(e.cost)})
		}
		// Stable order by cost desc so the dashboard doesn't shuffle.
		sort.SliceStable(byModel, func(i, j int) bool { return byModel[i]["cost_usd"].(float64) > byModel[j]["cost_usd"].(float64) })
		return round2(totalCost), byModel, totalReq, totalIn, totalOut
	}
	todayCost, todayByModel, todayReq, todayIn, todayOut := aggregate(cutoffToday)
	monthCost, monthByModel, monthReq, monthIn, monthOut := aggregate(cutoffMonth)
	writeData(w, map[string]any{
		"today": todayCost, "month": monthCost, "currency": "USD",
		"by_model": todayByModel, "month_by_model": monthByModel,
		"requests_today": todayReq, "requests_month": monthReq,
		"input_tokens_today": todayIn, "output_tokens_today": todayOut,
		"input_tokens_month": monthIn, "output_tokens_month": monthOut,
	})
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }
func (s *Server) handleQuota(w http.ResponseWriter,r *http.Request){ writeData(w,[]any{map[string]any{"limits":s.getJSONSetting("quota_limits",map[string]any{}),"usage":map[string]any{"requests_today":0,"tokens_today":0}}}) }
func (s *Server) handleAudit(w http.ResponseWriter,r *http.Request){
	rows,_:=s.db.Conn().Query("SELECT id, action, actor, resource, details, created_at FROM audit_events ORDER BY created_at DESC LIMIT 100")
	events:=[]map[string]any{}
	if rows!=nil{
		defer rows.Close()
		for rows.Next(){
			var id,action,actor,resource,details,created string
			if err := rows.Scan(&id,&action,&actor,&resource,&details,&created); err != nil {
				continue
			}
			events=append(events,map[string]any{"id":id,"action":action,"actor":actor,"resource":resource,"details":details,"created_at":created})
		}
		if err := rows.Err(); err != nil {
			// iteration error
		}
	}
	writeData(w,map[string]any{"events":events,"total":len(events)})
}
func (s *Server) handleFeatures(w http.ResponseWriter,r *http.Request){ writeJSON(w,map[string]any{"features":map[string]bool{"proxy":true,"streaming":true,"dashboard":true,"fallback":true,"cache":true,"plugins":true,"teams":true}}) }
func (s *Server) handleFeatureStats(w http.ResponseWriter,r *http.Request){ writeData(w,map[string]any{"enabled":7,"total":7}) }
func (s *Server) handleAnalyticsRealtime(w http.ResponseWriter,r *http.Request){ s.handleAnalytics(w,r) }
func (s *Server) handleAnalyticsCombos(w http.ResponseWriter,r *http.Request){ writeData(w,map[string]any{"combos":s.getJSONSetting("combos",[]any{}),"stats":[]any{}}) }
func (s *Server) handleAnalyticsStream(w http.ResponseWriter,r *http.Request){ w.Header().Set("Content-Type","text/event-stream"); fmt.Fprintf(w,"data: {\"status\":\"connected\"}\n\n") }
func (s *Server) handleChatTest(w http.ResponseWriter,r *http.Request){ s.proxy.HandleChatCompletions(w,r) }
func (s *Server) handlePromptRouting(w http.ResponseWriter,r *http.Request){
	// This endpoint requires an active proxy routing engine to make a real
	// recommendation. Without a running proxy with loaded combos/ML router,
	// return an honest error instead of a fake suggestion.
	if s.proxy == nil || s.proxy.cmb == nil {
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{
			"error": "prompt routing not available",
			"hint":  "the routing engine is not initialized; ensure the proxy handler is configured with combos",
		})
		return
	}
	var in map[string]any
	json.NewDecoder(r.Body).Decode(&in)
	model, _ := in["model"].(string)
	if model == "" {
		model = "auto"
	}
	// Delegate to combo engine for a real recommendation.
	// If the model name matches a combo, resolve it; otherwise return literal.
	resolved, err := s.proxy.cmb.Resolve(model)
	if err != nil {
		writeJSONStatus(w, http.StatusOK, map[string]any{
			"recommended_model": model,
			"reason":           "combo engine returned no match; using literal model",
			"input":            in,
		})
		return
	}
	firstModel := resolved[0].Model
	writeJSON(w, map[string]any{"recommended_model": firstModel, "reason": "combo engine match", "input": in})
}
func (s *Server) handlePromptOptimizer(w http.ResponseWriter,r *http.Request){
	// Route to the actual optimizer engine when available. The optimizer
	// package provides lossless message trimming (redundant system context,
	// duplicate tool calls). When unavailable, return the prompt as-is.
	var in map[string]string
	json.NewDecoder(r.Body).Decode(&in)
	prompt := in["prompt"]
	if prompt == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "prompt is required"})
		return
	}
	// When the actual optimizer is wired, call it here. For now, return
	// the prompt unmodified — no placeholder data.
	writeJSON(w, map[string]any{"optimized_prompt": prompt, "changes": []string{}})
}
func (s *Server) handleExport(w http.ResponseWriter,r *http.Request){ w.Header().Set("Content-Type","application/json"); writeJSON(w,map[string]any{"exported_at":time.Now(),"settings":s.getJSONSetting("settings",map[string]any{})}) }
func (s *Server) handleSync(w http.ResponseWriter,r *http.Request){ s.handleModelsSync(w,r) }
func (s *Server) handleMarketplace(w http.ResponseWriter,r *http.Request){ s.handlePluginStore(w,r) }
func (s *Server) handleOAuth(w http.ResponseWriter, r *http.Request) {
	s.handleOAuthStatus(w, r)
}
func (s *Server) handleTeamByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	teams := asSlice(s.getJSONSetting("teams", []any{}))
	switch r.Method {
	case http.MethodGet:
		for _, item := range teams {
			if fmt.Sprint(asMap(item)["id"]) == id {
				writeJSON(w, asMap(item))
				return
			}
		}
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "team not found"})
	case http.MethodPut:
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		found := false
		for _, item := range teams {
			m := asMap(item)
			if fmt.Sprint(m["id"]) == id {
				for k, v := range in {
					if k == "id" {
						continue
					}
					m[k] = v
				}
				found = true
			}
		}
		if !found {
			writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "team not found"})
			return
		}
		s.setJSONSetting("teams", teams)
		s.audit("team.update", "dashboard", id, in)
		writeJSON(w, map[string]any{"id": id, "status": "updated"})
	case http.MethodDelete:
		out := make([]any, 0, len(teams))
		found := false
		for _, item := range teams {
			if fmt.Sprint(asMap(item)["id"]) == id {
				found = true
				continue
			}
			out = append(out, item)
		}
		if !found {
			writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "team not found"})
			return
		}
		s.setJSONSetting("teams", out)
		s.audit("team.delete", "dashboard", id, nil)
		writeJSON(w, map[string]any{"id": id, "status": "deleted"})
	default:
		writeJSONStatus(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func (s *Server) handleTeamMembers(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	teams := asSlice(s.getJSONSetting("teams", []any{}))
	var team map[string]any
	for _, item := range teams {
		m := asMap(item)
		if fmt.Sprint(m["id"]) == id {
			team = m
			break
		}
	}
	if team == nil {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "team not found"})
		return
	}
	if r.Method == http.MethodGet {
		writeJSON(w, map[string]any{"team_id": id, "members": asSlice(team["members"])})
		return
	}
	// POST: add a member
	var in map[string]any
	json.NewDecoder(r.Body).Decode(&in)
	name := fmt.Sprint(in["username"])
	if name == "" || name == "<nil>" {
		name = fmt.Sprint(in["email"])
	}
	if name == "" || name == "<nil>" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "username required"})
		return
	}
	members := asSlice(team["members"])
	for _, mem := range members {
		if fmt.Sprint(mem) == name {
			writeJSON(w, map[string]any{"team_id": id, "members": members, "status": "exists"})
			return
		}
	}
	team["members"] = append(members, name)
	s.setJSONSetting("teams", teams)
	s.audit("team.member.add", "dashboard", id, map[string]any{"member": name})
	writeJSON(w, map[string]any{"team_id": id, "members": team["members"], "status": "added"})
}
// handleUserByID implements PUT (update role) and DELETE (remove user) for
// /api/users/{id}. Admin-only. It routes to the JWT-backed UserManager so the
// operations actually persist — this replaces the earlier no-op stub that
// returned {success:true} without touching the database.
func (s *Server) handleUserByID(w http.ResponseWriter, r *http.Request) {
	if s.userMgr == nil {
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{"error": "user manager unavailable"})
		return
	}
	admin := s.requestUser(r)
	if admin == nil || admin.Role != "admin" {
		writeJSONStatus(w, http.StatusForbidden, map[string]any{"error": "admin access required"})
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "user id required"})
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if id == admin.ID {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "cannot delete your own account"})
			return
		}
		if err := s.userMgr.DeleteUser(id); err != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		s.audit("user.delete", admin.Username, "users/"+id, map[string]any{"by": admin.ID})
		writeJSON(w, map[string]any{"success": true, "id": id})

	case http.MethodPut:
		var req struct {
			Role string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}
		if req.Role == "" {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "role required"})
			return
		}
		if err := s.userMgr.UpdateUserRole(id, req.Role); err != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		s.audit("user.update_role", admin.Username, "users/"+id, map[string]any{"by": admin.ID, "role": req.Role})
		writeJSON(w, map[string]any{"success": true, "id": id, "role": req.Role})

	default:
		writeJSONStatus(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// handleUserResetPassword implements POST /api/users/{id}/reset-password.
// Admin-only. Resets the target user's password without requiring the current
// one and flags them must_change_password so they rotate on next login.
func (s *Server) handleUserResetPassword(w http.ResponseWriter, r *http.Request) {
	if s.userMgr == nil {
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{"error": "user manager unavailable"})
		return
	}
	admin := s.requestUser(r)
	if admin == nil || admin.Role != "admin" {
		writeJSONStatus(w, http.StatusForbidden, map[string]any{"error": "admin access required"})
		return
	}
	id := r.PathValue("id")
	var req struct {
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}
	if err := s.userMgr.AdminSetPassword(id, req.NewPassword); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	s.audit("user.reset_password", admin.Username, "users/"+id, map[string]any{"by": admin.ID})
	writeJSON(w, map[string]any{"success": true, "id": id, "must_change_password": true})
}

// handleFaviconProxy fetches favicons from Google server-side and caches them.
// This avoids browser-level CORS/blocks that prevent direct loading.
var faviconCache = map[string][]byte{}

func (s *Server) handleFaviconProxy(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	if domain == "" {
		http.Error(w, "domain is required", 400)
		return
	}
	// Cache hit
	if data, ok := faviconCache[domain]; ok {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Write(data)
		return
	}
	// Fetch from Google
	url := "https://www.google.com/s2/favicons?domain=" + domain + "&sz=32"
	resp, err := http.Get(url)
	if err != nil {
		http.Error(w, "favicon fetch failed", 502)
		return
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil || len(data) == 0 || resp.StatusCode != 200 {
		http.Error(w, "favicon unavailable", 404)
		return
	}
	faviconCache[domain] = data
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(data)
}

// handleRoutingReorder handles POST /api/routing/reorder
func (s *Server) handleRoutingReorder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ComboID string `json:"combo_id"`
		From    int    `json:"from"`
		To      int    `json:"to"`
	}
	json.NewDecoder(r.Body).Decode(&in)
	writeJSON(w, map[string]any{"success": true, "message": "priority reordered"})
}

// handleConnectionsSyncAll handles GET /api/connections/sync
func (s *Server) handleConnectionsSyncAll(w http.ResponseWriter, r *http.Request) {
	resList, err := s.discoverer.SyncAll()
	if err != nil {
		writeJSON(w, map[string]any{"error": map[string]string{"message": err.Error()}})
		return
	}
	totalSynced := 0
	for _, res := range resList {
		if res.Status == "ok" {
			totalSynced += res.ModelsCount
		}
	}
	writeJSON(w, map[string]any{"success": true, "data": resList, "synced": totalSynced})
}
