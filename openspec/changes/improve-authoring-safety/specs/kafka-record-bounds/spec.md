## ADDED Requirements

### Requirement: Match accepted record bounds in every append path
Append, AppendBatch and Commit SHALL support one accepted maximal key and value plus encoding overhead. Ordinary producer batches SHALL retain their original 1,000,012-byte cap. Large plain appends SHALL be isolated from other records; transactional production SHALL use conservative chunks and send individually large records alone while preserving one atomic commit. Broker and topic limits remain separately required; clients MUST NOT claim to override them.

#### Scenario: Large configured topic
- **WHEN** a topic permits a valid record larger than one MiB
- **THEN** Append, AppendBatch and Commit can roundtrip that record through Kafka

#### Scenario: Ordinary records on an unchanged topic
- **WHEN** several individually valid records could combine past a topic's ordinary message-size limit
- **THEN** Append, AppendBatch and Commit preserve the original client batch cap instead of coalescing them into an oversized batch

#### Scenario: Concurrent large plain appends
- **WHEN** concurrent callers append individually large records to a topic that accepts each record
- **THEN** each record is sent alone rather than combined with another large record
