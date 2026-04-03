package planner

type StepNode struct {
	Step   Step
	Ready  bool
	Done   bool
	Result *StepResult
}

type StepResult struct {
	Success bool
	Data    string
	Error   string
}

type ExecutionGraph struct {
	nodes map[string]*StepNode
	edges map[string][]string
}

func NewExecutionGraph(steps []Step) *ExecutionGraph {
	g := &ExecutionGraph{
		nodes: make(map[string]*StepNode),
		edges: make(map[string][]string),
	}

	for _, step := range steps {
		g.nodes[step.ID] = &StepNode{Step: step}
		for _, dep := range step.DependsOn {
			g.edges[dep] = append(g.edges[dep], step.ID)
		}
	}

	g.markReady()
	return g
}

func (g *ExecutionGraph) TopologicalLevels() [][]Step {
	var levels [][]Step
	visited := make(map[string]bool)

	for {
		var level []Step
		for id, node := range g.nodes {
			if visited[id] {
				continue
			}
			if g.allDepsDone(id) {
				level = append(level, node.Step)
				visited[id] = true
			}
		}

		if len(level) == 0 {
			break
		}

		levels = append(levels, level)
		for _, step := range level {
			g.markDone(step.ID)
		}
	}

	return levels
}

func (g *ExecutionGraph) MarkResult(stepID string, result *StepResult) {
	if node, ok := g.nodes[stepID]; ok {
		node.Done = true
		node.Result = result
		g.markReady()
	}
}

func (g *ExecutionGraph) IsComplete() bool {
	for _, node := range g.nodes {
		if !node.Done {
			return false
		}
	}
	return true
}

func (g *ExecutionGraph) allDepsDone(stepID string) bool {
	node, ok := g.nodes[stepID]
	if !ok {
		return false
	}
	for _, dep := range node.Step.DependsOn {
		if depNode, ok := g.nodes[dep]; !ok || !depNode.Done {
			return false
		}
	}
	return true
}

func (g *ExecutionGraph) markReady() {
	for id, node := range g.nodes {
		if node.Done {
			continue
		}
		node.Ready = g.allDepsDone(id)
	}
}

func (g *ExecutionGraph) markDone(stepID string) {
	if node, ok := g.nodes[stepID]; ok {
		node.Done = true
	}
}

func (g *ExecutionGraph) ReadySteps() []Step {
	var steps []Step
	for _, node := range g.nodes {
		if node.Ready && !node.Done {
			steps = append(steps, node.Step)
		}
	}
	return steps
}
