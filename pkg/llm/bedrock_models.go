package llm

type BedrockModel struct {
	ID         string
	Name       string
	Provider   string
	InputCost  float64
	OutputCost float64
	ContextLen int
	FreeTier   bool
}

func (m BedrockModel) IsFree() bool {
	return m.FreeTier || (m.InputCost == 0 && m.OutputCost == 0)
}

func BedrockModels() []BedrockModel {
	return []BedrockModel{
		{
			ID:         "us.anthropic.claude-sonnet-4-20250514-v1:0",
			Name:       "Claude Sonnet 4",
			Provider:   "Anthropic",
			InputCost:  3.00,
			OutputCost: 15.00,
			ContextLen: 200000,
		},
		{
			ID:         "us.anthropic.claude-opus-4-20250514-v1:0",
			Name:       "Claude Opus 4",
			Provider:   "Anthropic",
			InputCost:  15.00,
			OutputCost: 75.00,
			ContextLen: 200000,
		},
		{
			ID:         "us.anthropic.claude-3-5-sonnet-20241022-v2:0",
			Name:       "Claude 3.5 Sonnet v2",
			Provider:   "Anthropic",
			InputCost:  3.00,
			OutputCost: 15.00,
			ContextLen: 200000,
		},
		{
			ID:         "us.anthropic.claude-3-5-haiku-20241022-v1:0",
			Name:       "Claude 3.5 Haiku",
			Provider:   "Anthropic",
			InputCost:  0.80,
			OutputCost: 4.00,
			ContextLen: 200000,
		},
		{
			ID:         "us.anthropic.claude-3-haiku-20240307-v1:0",
			Name:       "Claude 3 Haiku",
			Provider:   "Anthropic",
			InputCost:  0.25,
			OutputCost: 1.25,
			ContextLen: 200000,
		},
		{
			ID:         "us.meta.llama4-scout-17b-instruct-v1:0",
			Name:       "Llama 4 Scout",
			Provider:   "Meta",
			InputCost:  0.15,
			OutputCost: 0.60,
			ContextLen: 128000,
		},
		{
			ID:         "us.meta.llama4-maverick-17b-instruct-v1:0",
			Name:       "Llama 4 Maverick",
			Provider:   "Meta",
			InputCost:  0.20,
			OutputCost: 0.80,
			ContextLen: 128000,
		},
		{
			ID:         "us.meta.llama3-3-70b-instruct-v1:0",
			Name:       "Llama 3.3 70B",
			Provider:   "Meta",
			InputCost:  0.72,
			OutputCost: 0.72,
			ContextLen: 128000,
		},
		{
			ID:         "us.meta.llama3-2-11b-instruct-v1:0",
			Name:       "Llama 3.2 11B",
			Provider:   "Meta",
			InputCost:  0.16,
			OutputCost: 0.16,
			ContextLen: 128000,
		},
		{
			ID:         "us.meta.llama3-2-3b-instruct-v1:0",
			Name:       "Llama 3.2 3B",
			Provider:   "Meta",
			InputCost:  0.10,
			OutputCost: 0.10,
			ContextLen: 128000,
		},
		{
			ID:         "us.meta.llama3-2-1b-instruct-v1:0",
			Name:       "Llama 3.2 1B",
			Provider:   "Meta",
			InputCost:  0.10,
			OutputCost: 0.10,
			ContextLen: 128000,
		},
		{
			ID:         "us.amazon.nova-pro-v1:0",
			Name:       "Amazon Nova Pro",
			Provider:   "Amazon",
			InputCost:  0.80,
			OutputCost: 3.20,
			ContextLen: 300000,
		},
		{
			ID:         "us.amazon.nova-lite-v1:0",
			Name:       "Amazon Nova Lite",
			Provider:   "Amazon",
			InputCost:  0.06,
			OutputCost: 0.24,
			ContextLen: 300000,
		},
		{
			ID:         "us.amazon.nova-micro-v1:0",
			Name:       "Amazon Nova Micro",
			Provider:   "Amazon",
			InputCost:  0.035,
			OutputCost: 0.14,
			ContextLen: 128000,
			FreeTier:   true,
		},
		{
			ID:         "cohere.command-r-plus-v1:0",
			Name:       "Command R+",
			Provider:   "Cohere",
			InputCost:  2.50,
			OutputCost: 10.00,
			ContextLen: 128000,
		},
		{
			ID:         "cohere.command-r-v1:0",
			Name:       "Command R",
			Provider:   "Cohere",
			InputCost:  0.50,
			OutputCost: 1.50,
			ContextLen: 128000,
		},
		{
			ID:         "usmistral.mistral-large-2407-v1:0",
			Name:       "Mistral Large 2",
			Provider:   "Mistral",
			InputCost:  2.00,
			OutputCost: 6.00,
			ContextLen: 128000,
		},
		{
			ID:         "us.mistral.mistral-small-2402-v1:0",
			Name:       "Mistral Small",
			Provider:   "Mistral",
			InputCost:  0.20,
			OutputCost: 0.60,
			ContextLen: 32000,
		},
		{
			ID:         "us.ai21.jamba-1-5-large-v1:0",
			Name:       "Jamba 1.5 Large",
			Provider:   "AI21",
			InputCost:  2.00,
			OutputCost: 8.00,
			ContextLen: 256000,
		},
		{
			ID:         "us.ai21.jamba-1-5-mini-v1:0",
			Name:       "Jamba 1.5 Mini",
			Provider:   "AI21",
			InputCost:  0.20,
			OutputCost: 0.40,
			ContextLen: 256000,
		},
		{
			ID:         "us.deepseek.r1-v1:0",
			Name:       "DeepSeek R1",
			Provider:   "DeepSeek",
			InputCost:  1.35,
			OutputCost: 5.40,
			ContextLen: 128000,
		},
	}
}

func BedrockModelByID(id string) (BedrockModel, bool) {
	for _, m := range BedrockModels() {
		if m.ID == id {
			return m, true
		}
	}
	return BedrockModel{}, false
}
