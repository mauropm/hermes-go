package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

type BedrockProvider struct {
	client  *bedrockruntime.Client
	region  string
	timeout time.Duration
}

func NewBedrockProvider(region string, profile string, accessKeyID string, secretAccessKey string, timeout time.Duration) (*BedrockProvider, error) {
	if region == "" {
		region = "us-east-1"
	}

	opts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}

	if profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(profile))
	}

	if accessKeyID != "" && secretAccessKey != "" {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
		))
	}

	cfg, err := config.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	if timeout == 0 {
		timeout = 60 * time.Second
	}

	return &BedrockProvider{
		client:  bedrockruntime.NewFromConfig(cfg),
		region:  region,
		timeout: timeout,
	}, nil
}

func (p *BedrockProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, model string) (*Response, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	var systemPrompt string
	var chatMessages []types.Message

	if messages[0].Role == "system" {
		systemPrompt = messages[0].Content
		messages = messages[1:]
	}

	for _, msg := range messages {
		bm := types.Message{
			Role: types.ConversationRole(msg.Role),
		}

		switch msg.Role {
		case "assistant":
			var content []types.ContentBlock
			if msg.Content != "" {
				content = append(content, &types.ContentBlockMemberText{
					Value: msg.Content,
				})
			}
			for _, tc := range msg.ToolCalls {
				var argsDoc document.Interface
				if tc.Function.Arguments != "" {
					argsDoc = document.NewLazyDocument(json.RawMessage(tc.Function.Arguments))
				} else {
					argsDoc = document.NewLazyDocument(map[string]interface{}{})
				}
				content = append(content, &types.ContentBlockMemberToolUse{
					Value: types.ToolUseBlock{
						ToolUseId: aws.String(tc.ID),
						Name:      aws.String(tc.Function.Name),
						Input:     argsDoc,
					},
				})
			}
			bm.Content = content

		case "tool":
			bm.Role = types.ConversationRoleUser
			bm.Content = []types.ContentBlock{
				&types.ContentBlockMemberToolResult{
					Value: types.ToolResultBlock{
						ToolUseId: aws.String(msg.ToolCallID),
						Content: []types.ToolResultContentBlock{
							&types.ToolResultContentBlockMemberText{
								Value: msg.Content,
							},
						},
					},
				},
			}

		default:
			bm.Content = []types.ContentBlock{
				&types.ContentBlockMemberText{
					Value: msg.Content,
				},
			}
		}

		chatMessages = append(chatMessages, bm)
	}

	var bedrockTools []types.Tool
	for _, t := range tools {
		bedrockTools = append(bedrockTools, &types.ToolMemberToolSpec{
			Value: types.ToolSpecification{
				Name:        aws.String(t.Function.Name),
				Description: aws.String(t.Function.Description),
				InputSchema: &types.ToolInputSchemaMemberJson{
					Value: document.NewLazyDocument(t.Function.Parameters),
				},
			},
		})
	}

	input := &bedrockruntime.ConverseInput{
		ModelId:  aws.String(model),
		Messages: chatMessages,
		InferenceConfig: &types.InferenceConfiguration{
			MaxTokens:   aws.Int32(4096),
			Temperature: aws.Float32(0.7),
			TopP:        aws.Float32(1.0),
		},
		ToolConfig: &types.ToolConfiguration{
			Tools: bedrockTools,
		},
	}

	if systemPrompt != "" {
		input.System = []types.SystemContentBlock{
			&types.SystemContentBlockMemberText{
				Value: systemPrompt,
			},
		}
	}

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	out, err := p.client.Converse(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("bedrock converse: %w", err)
	}

	result := &Response{
		FinishReason: string(out.StopReason),
	}

	if out.Usage != nil {
		result.Usage = Usage{
			InputTokens:  int(aws.ToInt32(out.Usage.InputTokens)),
			OutputTokens: int(aws.ToInt32(out.Usage.OutputTokens)),
		}
	}

	msgOut, ok := out.Output.(*types.ConverseOutputMemberMessage)
	if !ok {
		return nil, fmt.Errorf("unexpected output type: %T", out.Output)
	}

	for _, cb := range msgOut.Value.Content {
		switch v := cb.(type) {
		case *types.ContentBlockMemberText:
			result.Content = v.Value
		case *types.ContentBlockMemberToolUse:
			argsJSON, err := json.Marshal(v.Value.Input)
			if err != nil {
				argsJSON = []byte("{}")
			}
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:   aws.ToString(v.Value.ToolUseId),
				Type: "function",
				Function: FunctionCall{
					Name:      aws.ToString(v.Value.Name),
					Arguments: string(argsJSON),
				},
			})
		}
	}

	return result, nil
}
