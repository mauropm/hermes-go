package tool

import (
	"context"
	"fmt"
	"sync"
)

type ParallelExecutor struct {
	maxWorkers int
	registry   *Registry
}

func NewParallelExecutor(registry *Registry, maxWorkers int) *ParallelExecutor {
	if maxWorkers <= 0 {
		maxWorkers = 4
	}
	return &ParallelExecutor{
		maxWorkers: maxWorkers,
		registry:   registry,
	}
}

type ExecutionPlan struct {
	Sequential []string
	Parallel   [][]string
}

func (pe *ParallelExecutor) Execute(ctx context.Context, plan ExecutionPlan, inputs map[string]ToolInput) ([]ToolOutput, error) {
	var results []ToolOutput

	for _, name := range plan.Sequential {
		t, ok := pe.registry.Get(name)
		if !ok {
			return results, fmt.Errorf("tool %q not found", name)
		}
		in := inputs[name]
		out, err := t.Execute(ctx, in)
		if err != nil {
			return results, fmt.Errorf("tool %s: %w", name, err)
		}
		results = append(results, out)
	}

	for _, group := range plan.Parallel {
		type result struct {
			idx int
			out ToolOutput
			err error
		}
		ch := make(chan result, len(group))

		var wg sync.WaitGroup
		sem := make(chan struct{}, pe.maxWorkers)

		for i, name := range group {
			wg.Add(1)
			go func(idx int, toolName string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				t, ok := pe.registry.Get(toolName)
				if !ok {
					ch <- result{idx, ToolOutput{}, fmt.Errorf("tool %q not found", toolName)}
					return
				}
				in := inputs[toolName]
				out, err := t.Execute(ctx, in)
				ch <- result{idx, out, err}
			}(i, name)
		}

		wg.Wait()
		close(ch)

		ordered := make([]ToolOutput, len(group))
		for r := range ch {
			if r.err != nil {
				return results, fmt.Errorf("parallel tool %s: %w", group[r.idx], r.err)
			}
			ordered[r.idx] = r.out
		}
		results = append(results, ordered...)
	}

	return results, nil
}
