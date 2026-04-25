package fault

type LogEntry struct {
	SenderID string 
	Seq int64 
	Payload []byte 
	Signature []byte
}

type EquivocationChecker interface {
	DetectEquivocation() (senderID string, a, b *LogEntry)
}