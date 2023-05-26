package task

type CommandInfo struct {
	Command string
	Result   string
}

type CommandBatchInfo struct {
	Items []CommandInfo
}
