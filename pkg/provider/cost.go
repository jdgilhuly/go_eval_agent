package provider

// modelPricing holds per-million-token pricing for known models.
type modelPricing struct {
	InputPerMillion  float64
	OutputPerMillion float64
}

// pricing maps model identifiers to their token costs in USD.
var pricing = map[string]modelPricing{
	// Claude 3 family
	"claude-3-opus-20240229":   {InputPerMillion: 15.0, OutputPerMillion: 75.0},
	"claude-3-sonnet-20240229": {InputPerMillion: 3.0, OutputPerMillion: 15.0},
	"claude-3-haiku-20240307":  {InputPerMillion: 0.25, OutputPerMillion: 1.25},

	// Claude 3.5 family
	"claude-3-5-sonnet-20241022": {InputPerMillion: 3.0, OutputPerMillion: 15.0},
	"claude-3-5-haiku-20241022":  {InputPerMillion: 0.80, OutputPerMillion: 4.0},

	// Claude 4 family
	"claude-sonnet-4-5-20250929": {InputPerMillion: 3.0, OutputPerMillion: 15.0},
	"claude-opus-4-6":            {InputPerMillion: 15.0, OutputPerMillion: 75.0},

	// OpenAI GPT-4o family
	"gpt-4o":      {InputPerMillion: 2.50, OutputPerMillion: 10.0},
	"gpt-4o-mini": {InputPerMillion: 0.15, OutputPerMillion: 0.60},

	// OpenAI GPT-4 family
	"gpt-4-turbo": {InputPerMillion: 10.0, OutputPerMillion: 30.0},
	"gpt-4":       {InputPerMillion: 30.0, OutputPerMillion: 60.0},

	// OpenAI o-series
	"o1":      {InputPerMillion: 15.0, OutputPerMillion: 60.0},
	"o1-mini": {InputPerMillion: 3.0, OutputPerMillion: 12.0},
	"o3-mini": {InputPerMillion: 1.10, OutputPerMillion: 4.40},
}

// EstimateCost returns the estimated USD cost for the given model and usage.
// Returns 0 if the model is not in the pricing table.
func EstimateCost(model string, usage Usage) float64 {
	p, ok := pricing[model]
	if !ok {
		return 0
	}
	inputCost := float64(usage.InputTokens) / 1_000_000 * p.InputPerMillion
	outputCost := float64(usage.OutputTokens) / 1_000_000 * p.OutputPerMillion
	return inputCost + outputCost
}
