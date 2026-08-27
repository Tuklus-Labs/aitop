package graph

import "math"

type chargeResult struct {
	bytes uint64
	ok    bool
}

func validCharge(bytes uint64) chargeResult { return chargeResult{bytes: bytes, ok: true} }
func saturatedCharge() chargeResult         { return chargeResult{bytes: math.MaxUint64} }

func logicalAdd(left, right uint64) chargeResult {
	if right > math.MaxUint64-left {
		return saturatedCharge()
	}
	return validCharge(left + right)
}

func addCharges(values ...chargeResult) chargeResult {
	total := validCharge(0)
	for _, value := range values {
		if !total.ok || !value.ok {
			return saturatedCharge()
		}
		total = logicalAdd(total.bytes, value.bytes)
	}
	return total
}

func subtractCharge(total chargeResult, bytes uint64) chargeResult {
	if !total.ok || total.bytes < bytes {
		return saturatedCharge()
	}
	return validCharge(total.bytes - bytes)
}

func logicalMultiply(left, right uint64) chargeResult {
	if left != 0 && right > math.MaxUint64/left {
		return saturatedCharge()
	}
	return validCharge(left * right)
}

func logicalAlign(bytes uint64) chargeResult {
	if bytes == 0 {
		return validCharge(0)
	}
	if bytes > math.MaxUint64-15 {
		return saturatedCharge()
	}
	return validCharge((bytes + 15) &^ 15)
}

func logicalNonnegativeInt(value int) chargeResult {
	if value < 0 {
		return saturatedCharge()
	}
	return validCharge(uint64(value))
}

func logicalString(length int) chargeResult {
	converted := logicalNonnegativeInt(length)
	if !converted.ok {
		return converted
	}
	return addCharges(validCharge(16), logicalAlign(converted.bytes))
}

func logicalPointer(present bool) chargeResult {
	if !present {
		return validCharge(0)
	}
	return validCharge(32)
}

func logicalSlice(count int, fixedElementBytes uint64) chargeResult {
	converted := logicalNonnegativeInt(count)
	if !converted.ok {
		return converted
	}
	storage := logicalMultiply(converted.bytes, fixedElementBytes)
	if !storage.ok {
		return storage
	}
	return addCharges(validCharge(32), logicalAlign(storage.bytes))
}

type logicalMapCharge struct{ chargeResult }

func newLogicalMapCharge() logicalMapCharge {
	return logicalMapCharge{chargeResult: validCharge(64)}
}

func (m logicalMapCharge) add(key, value chargeResult) logicalMapCharge {
	m.chargeResult = addCharges(m.chargeResult, validCharge(64), key, value)
	return m
}

func (m logicalMapCharge) total() chargeResult { return m.chargeResult }

func chargeProcessIdentity(ProcessIdentity) chargeResult { return validCharge(32) }
func chargeTrace(TraceID) chargeResult                   { return validCharge(32) }
func chargeTokenUsage(TokenUsage) chargeResult           { return validCharge(64) }

func chargeSourceRef(source SourceRef) chargeResult {
	return addCharges(validCharge(96), logicalString(len(source.ID)))
}

func chargeTransition(value Transition) chargeResult {
	return addCharges(validCharge(96), logicalString(len(value.Source.ID)))
}

func chargeGap(value Gap) chargeResult {
	return addCharges(validCharge(128), logicalString(len(value.Source)), logicalPointer(value.Capability != nil))
}

func chargeObservationCursor(observationCursor) chargeResult { return validCharge(160) }

func chargeSequenceRecord(value sequenceRecord) chargeResult {
	result := addCharges(validCharge(256), logicalSlice(len(value.buffered), 512), logicalSlice(len(value.missing), 32))
	for _, event := range value.buffered {
		dynamic := chargeEventDynamic(event)
		if !dynamic.ok {
			return dynamic
		}
		result = addCharges(result, dynamic)
	}
	return result
}

func chargeMetricsValue(metrics Metrics) chargeResult {
	result := addCharges(validCharge(64), logicalString(len(metrics.CostSource)))
	if metrics.Usage != nil {
		result = addCharges(result, validCharge(32), chargeTokenUsage(*metrics.Usage))
	}
	for _, present := range []bool{
		metrics.TokenRate != nil,
		metrics.ContextUsed != nil,
		metrics.ContextWindow != nil,
		metrics.ContextFill != nil,
		metrics.CacheUse != nil,
		metrics.CostUSD != nil,
	} {
		result = addCharges(result, logicalPointer(present))
	}
	return result
}

func chargeNodeRecord(node Node) chargeResult {
	result := validCharge(512)
	for _, value := range []string{
		string(node.ID), string(node.Incarnation), node.ProvenName, node.Model,
		node.Project, node.Worktree, node.TaskName, string(node.State.Source.ID),
	} {
		result = addCharges(result, logicalString(len(value)))
	}
	if node.Process != nil {
		result = addCharges(result, validCharge(64))
	}
	for _, present := range []bool{
		node.StartedAt != nil, node.CompletedAt != nil, node.FailedAt != nil, node.GhostExpiresAt != nil,
	} {
		result = addCharges(result, logicalPointer(present))
	}
	metrics := chargeMetricsValue(node.Metrics)
	if !metrics.ok || metrics.bytes < 64 {
		return saturatedCharge()
	}
	return addCharges(result, validCharge(metrics.bytes-64))
}

func chargePublishedNode(node Node) chargeResult {
	result := chargeNodeRecord(node)
	if node.Transitions != nil {
		result = addCharges(result, logicalSlice(cap(node.Transitions), 96))
		for _, transition := range node.Transitions {
			result = addCharges(result, logicalString(len(transition.Source.ID)))
		}
	}
	return result
}

func chargeEdgeRecord(record edgeRecord) chargeResult {
	if (record.relationships == nil) == (record.messages == nil) {
		return saturatedCharge()
	}
	edge := record.value
	result := validCharge(384)
	for _, value := range []string{
		string(edge.Key), string(edge.Source), string(edge.Target), string(edge.Relationship),
		string(record.sourceIncarnation), string(record.targetIncarnation),
	} {
		result = addCharges(result, logicalString(len(value)))
	}
	if edge.Trace != nil {
		result = addCharges(result, validCharge(64))
	}
	if edge.Delivery != nil {
		result = addCharges(result, validCharge(32+64+5*16))
	}
	result = addCharges(result, validCharge(64))
	for key, value := range record.relationships {
		result = addCharges(result, chargeRelationshipEntry(key, value))
	}
	for _, value := range record.messages {
		result = addCharges(result, chargeMessageEntry(value))
	}
	return result
}

func chargePublicEdge(edge Edge) chargeResult {
	result := validCharge(384)
	for _, value := range []string{string(edge.Key), string(edge.Source), string(edge.Target), string(edge.Relationship)} {
		result = addCharges(result, logicalString(len(value)))
	}
	if edge.Trace != nil {
		result = addCharges(result, validCharge(64))
	}
	if edge.Delivery != nil {
		result = addCharges(result, validCharge(32+64+5*16))
	}
	return result
}

func chargeEvent(event Event) chargeResult {
	return addCharges(validCharge(512), chargeEventDynamic(event))
}

func chargeEventDynamic(event Event) chargeResult {
	result := validCharge(0)
	for _, value := range []string{
		string(event.Source.Ref.ID), string(event.Actor), string(event.ActorIncarnation),
		string(event.Target), string(event.TargetIncarnation),
	} {
		result = addCharges(result, logicalString(len(value)))
	}
	result = addCharges(result, logicalPointer(event.Sequence != nil), logicalPointer(event.SourceTime != nil))
	if event.Trace != nil {
		result = addCharges(result, validCharge(64))
	}
	if event.Observation != nil {
		result = addCharges(result, validCharge(32+64+16+32), logicalString(len(event.Observation.Key)))
	}
	return addCharges(result, chargePayloadDynamic(event.Data))
}

func chargePayloadDynamic(data EventData) chargeResult {
	switch value := data.(type) {
	case NodeObserved:
		result := validCharge(64 + 2*16)
		for _, field := range []string{value.ProvenName, value.Model, value.Project, value.Worktree, value.TaskName} {
			result = addCharges(result, logicalString(len(field)))
		}
		if value.Process != nil {
			result = addCharges(result, validCharge(64))
		}
		return addCharges(result, logicalPointer(value.StartedAt != nil))
	case MetricsObserved:
		return chargeMetricsValue(value.Metrics)
	case StateObserved:
		return addCharges(validCharge(64+2*16), logicalString(len(value.Relationship)))
	case RelationshipObserved:
		return addCharges(validCharge(64+2*16), logicalString(len(value.Relationship)))
	case MessageObserved:
		return addCharges(validCharge(64+2*16), logicalString(len(value.Relationship)))
	case ExitObserved:
		return validCharge(64 + 16)
	case HeartbeatObserved:
		return validCharge(64)
	case GapObserved:
		return validCharge(64 + 4*16)
	case LaunchIntentObserved:
		result := validCharge(64 + 16)
		if value.ChildProcess != nil {
			result = addCharges(result, validCharge(64))
		}
		return result
	case SessionBindObserved:
		return validCharge(64 + 16 + 32)
	default:
		return saturatedCharge()
	}
}

type retainedOwner uint8

type retainedOwnerDefinition struct {
	owner retainedOwner
	name  string
	field string
}

const (
	ownerNodes retainedOwner = iota + 1
	ownerNodeContributions
	ownerMetricContributions
	ownerStateContributions
	ownerEdges
	ownerFingerprints
	ownerObservationCursors
	ownerCurrentIncarnations
	ownerRetiredIncarnations
	ownerSequenceRecords
	ownerApprovalRelationships
	ownerMessageExpiryIndex
	ownerGaps
	ownerTransitions
	ownerHealthEpochs
)

func retainedOwnerDefinitions() []retainedOwnerDefinition {
	return []retainedOwnerDefinition{
		{ownerNodes, "nodes", "nodes"},
		{ownerNodeContributions, "node-field-contributions", "nodeContributions"},
		{ownerMetricContributions, "metric-contributions", "metricContributions"},
		{ownerStateContributions, "state-contributions", "stateContributions"},
		{ownerEdges, "edges", "edges"},
		{ownerFingerprints, "fingerprints", "fingerprints"},
		{ownerObservationCursors, "observation-cursors", "observationCursors"},
		{ownerCurrentIncarnations, "current-incarnations", "currentIncarnations"},
		{ownerRetiredIncarnations, "retired-incarnations", "retiredIncarnations"},
		{ownerSequenceRecords, "sequence-records", "sequenceRecords"},
		{ownerApprovalRelationships, "approval-relationships", "approvalRelationships"},
		{ownerMessageExpiryIndex, "message-expiry-index", "messageExpiryIndex"},
		{ownerGaps, "gaps", "gaps"},
		{ownerTransitions, "transitions", "transitions"},
		{ownerHealthEpochs, "health-epochs", "healthEpochs"},
	}
}

func retainedOwners() []retainedOwner {
	definitions := retainedOwnerDefinitions()
	owners := make([]retainedOwner, len(definitions))
	for index, definition := range definitions {
		owners[index] = definition.owner
	}
	return owners
}

func (owner retainedOwner) name() string {
	for _, definition := range retainedOwnerDefinitions() {
		if definition.owner == owner {
			return definition.name
		}
	}
	return ""
}

func retainedOwnerNames() []string {
	owners := retainedOwners()
	names := make([]string, len(owners))
	for index, owner := range owners {
		names[index] = owner.name()
	}
	return names
}

func chargeEmptyRetainedRoot() chargeResult { return validCharge(64 + 4*16 + 15*64) }

func chargeSnapshot(snapshot *Snapshot) chargeResult {
	if snapshot == nil {
		return saturatedCharge()
	}
	result := validCharge(64 + 5*16)
	result = addCharges(result, logicalSlice(len(snapshot.Nodes), 512))
	for _, node := range snapshot.Nodes {
		charge := chargeNodeRecord(node)
		if !charge.ok || charge.bytes < 512 {
			return saturatedCharge()
		}
		result = addCharges(result, validCharge(charge.bytes-512))
		if node.Transitions != nil {
			transitionStorage := logicalSlice(cap(node.Transitions), 96)
			if len(node.Transitions) > cap(node.Transitions) {
				return saturatedCharge()
			}
			result = addCharges(result, transitionStorage)
			for _, transition := range node.Transitions {
				result = addCharges(result, logicalString(len(transition.Source.ID)))
			}
		}
	}
	result = addCharges(result, logicalSlice(len(snapshot.Edges), 384))
	for _, edge := range snapshot.Edges {
		charge := chargePublicEdge(edge)
		if !charge.ok || charge.bytes < 384 {
			return saturatedCharge()
		}
		result = addCharges(result, validCharge(charge.bytes-384))
	}
	result = addCharges(result, logicalSlice(len(snapshot.Gaps), 128))
	for _, gap := range snapshot.Gaps {
		charge := chargeGap(gap)
		if !charge.ok || charge.bytes < 128 {
			return saturatedCharge()
		}
		result = addCharges(result, validCharge(charge.bytes-128))
	}
	return result
}

func chargeContributionKey(key contributionKey) chargeResult {
	return addCharges(validCharge(64), logicalString(len(key.actor)), logicalString(len(key.incarnation)), chargeSourceRef(key.source.Ref), validCharge(16))
}

func chargeContributionOrder(contributionOrder) chargeResult { return validCharge(32) }

func chargeNodeContribution(value nodeFieldContribution) chargeResult {
	result := addCharges(validCharge(64+2*16), chargeContributionOrder(value.order))
	for _, field := range []optionalString{value.provenName, value.model, value.project, value.worktree, value.taskName} {
		result = addCharges(result, logicalString(len(field.value)))
	}
	if value.process != nil {
		result = addCharges(result, validCharge(64))
	}
	return addCharges(result, logicalPointer(value.startedAt != nil))
}

func chargeFingerprintEntry() chargeResult { return validCharge(64 + 32 + 32) }

func chargeNodeEntry(id NodeID, value *nodeRecord) chargeResult {
	if value == nil {
		return validCharge(0)
	}
	return addCharges(validCharge(64), logicalString(len(id)), chargeNodeRecord(value.value))
}

func chargeNodeContributionEntry(key contributionKey, value nodeFieldContribution) chargeResult {
	return addCharges(validCharge(64), chargeContributionKey(key), chargeNodeContribution(value))
}

func chargeMetricsContributionEntry(key contributionKey, value metricsContribution) chargeResult {
	return addCharges(validCharge(64), chargeContributionKey(key), validCharge(64), chargeContributionOrder(value.order), chargeMetricsValue(value.metrics))
}

func chargeStateContributionEntry(key contributionKey, value stateContribution) chargeResult {
	state := addCharges(validCharge(64+3*16), chargeContributionOrder(value.order), logicalString(len(value.evidence.Relationship)), logicalPointer(value.evidence.Sequence != nil))
	return addCharges(validCharge(64), chargeContributionKey(key), state)
}

func chargeStableSourceKey(key stableSourceKey) chargeResult {
	return addCharges(validCharge(80), logicalString(len(key.id)))
}

func chargeCursorEntry(key cursorKey, value observationCursor) chargeResult {
	cursorKeyCharge := addCharges(validCharge(64), chargeStableSourceKey(key.source), logicalString(len(key.observation)))
	return addCharges(validCharge(64), cursorKeyCharge, chargeObservationCursor(value))
}

func chargeCurrentIncarnationEntry(actor NodeID, value incarnationRecord) chargeResult {
	key := addCharges(validCharge(64), logicalString(len(actor)))
	record := addCharges(validCharge(64), logicalString(len(value.incarnation)), logicalPointer(value.startedAt != nil), logicalPointer(value.startTicks != nil))
	return addCharges(validCharge(64), key, record)
}

func chargeRetiredProofEntry(key retiredProofKey, value incarnationProof) chargeResult {
	keyCharge := addCharges(validCharge(64), logicalString(len(key.actor)), logicalString(len(key.incarnation)))
	valueCharge := addCharges(validCharge(64), logicalPointer(value.startedAt != nil), logicalPointer(value.startTicks != nil))
	return addCharges(validCharge(64), keyCharge, valueCharge)
}

func chargeGapKey(key gapKey) chargeResult {
	return addCharges(validCharge(64+16), logicalString(len(key.source)), logicalPointer(key.capabilityPresent))
}

func chargeActiveGapEntry(key gapKey, value Gap) chargeResult {
	return addCharges(validCharge(64), chargeGapKey(key), chargeGap(value))
}

func chargeTransitionEntry(actor NodeID, values []Transition, limit int) chargeResult {
	if len(values) > limit {
		return saturatedCharge()
	}
	slice := logicalSlice(limit, 96)
	for _, value := range values {
		slice = addCharges(slice, logicalString(len(value.Source.ID)))
	}
	return addCharges(validCharge(64), logicalString(len(actor)), slice)
}

func chargeRelationshipEntry(key relationshipContributionKey, _ relationshipContribution) chargeResult {
	keyCharge := addCharges(validCharge(64+16), chargeSourceRef(key.source))
	return addCharges(validCharge(64), keyCharge, validCharge(64+4*16))
}

func chargeMessageEntry(value messageContribution) chargeResult {
	return addCharges(validCharge(64+32), validCharge(64+3*16), chargeSourceRef(value.source))
}

func chargeEdgeEntry(key EdgeKey, value *edgeRecord) chargeResult {
	if value == nil {
		return validCharge(0)
	}
	return addCharges(validCharge(64), logicalString(len(key)), chargeEdgeRecord(*value))
}

func chargeSequenceKey(key sequenceKey) chargeResult {
	return addCharges(validCharge(64), logicalString(len(key.actor)), logicalString(len(key.incarnation)), chargeSourceRef(key.source))
}

func chargeSequenceEntry(key sequenceKey, value *sequenceRecord) chargeResult {
	if value == nil {
		return validCharge(0)
	}
	return addCharges(validCharge(64), chargeSequenceKey(key), chargeSequenceRecord(*value))
}

func chargeApprovalKey(key approvalKey) chargeResult {
	return addCharges(validCharge(64+16), logicalString(len(key.actor)), logicalString(len(key.incarnation)), chargeSourceRef(key.source.Ref), logicalString(len(key.relationship)))
}

func chargeApprovalEntry(key approvalKey, value *stateContribution) chargeResult {
	if value == nil {
		return validCharge(0)
	}
	state := addCharges(validCharge(64+3*16), chargeContributionOrder(value.order), logicalString(len(value.evidence.Relationship)), logicalPointer(value.evidence.Sequence != nil))
	return addCharges(validCharge(64), chargeApprovalKey(key), state)
}

func chargeMessageExpiryEntry(key messageExpiryKey, value *messageExpiryRef) chargeResult {
	if value == nil {
		return validCharge(0)
	}
	keyCharge := validCharge(64 + 16 + 32)
	return addCharges(validCharge(64), keyCharge, validCharge(16))
}

func chargeRetainedRoot(r *Reconciler, txn *reconcileTxn) chargeResult {
	if r == nil {
		return saturatedCharge()
	}
	result := chargeEmptyRetainedRoot()
	add := func(charge chargeResult) {
		result = addCharges(result, charge)
	}

	for key, value := range r.nodes {
		if txn != nil && txn.nodes != nil {
			if replacement, exists := txn.nodes[key]; exists {
				value = replacement
			}
		}
		add(chargeNodeEntry(key, value))
	}
	if txn != nil {
		for key, value := range txn.nodes {
			if _, exists := r.nodes[key]; !exists {
				add(chargeNodeEntry(key, value))
			}
		}
	}

	for key, value := range r.nodeContributions {
		if txn != nil && txn.nodeContributions != nil {
			if replacement, exists := txn.nodeContributions[key]; exists {
				value = replacement
			}
		}
		if value != nil {
			add(chargeNodeContributionEntry(key, *value))
		}
	}
	if txn != nil {
		for key, value := range txn.nodeContributions {
			if _, exists := r.nodeContributions[key]; !exists && value != nil {
				add(chargeNodeContributionEntry(key, *value))
			}
		}
	}

	for key, value := range r.metricContributions {
		if txn != nil && txn.metricContributions != nil {
			if replacement, exists := txn.metricContributions[key]; exists {
				value = replacement
			}
		}
		if value != nil {
			add(chargeMetricsContributionEntry(key, *value))
		}
	}
	if txn != nil {
		for key, value := range txn.metricContributions {
			if _, exists := r.metricContributions[key]; !exists && value != nil {
				add(chargeMetricsContributionEntry(key, *value))
			}
		}
	}

	for key, value := range r.stateContributions {
		if txn != nil && txn.stateContributions != nil {
			if replacement, exists := txn.stateContributions[key]; exists {
				value = replacement
			}
		}
		if value != nil {
			add(chargeStateContributionEntry(key, *value))
		}
	}
	if txn != nil {
		for key, value := range txn.stateContributions {
			if _, exists := r.stateContributions[key]; !exists && value != nil {
				add(chargeStateContributionEntry(key, *value))
			}
		}
	}
	for key, value := range r.edges {
		if txn != nil && txn.edges != nil {
			if replacement, exists := txn.edges[key]; exists {
				value = replacement
			}
		}
		add(chargeEdgeEntry(key, value))
	}
	if txn != nil {
		for key, value := range txn.edges {
			if _, exists := r.edges[key]; !exists {
				add(chargeEdgeEntry(key, value))
			}
		}
	}
	for key := range r.fingerprints {
		if txn != nil {
			if _, replacement := txn.fingerprints[key]; replacement {
				continue
			}
		}
		add(chargeFingerprintEntry())
	}
	if txn != nil {
		for key := range txn.fingerprints {
			if _, exists := r.fingerprints[key]; !exists {
				add(chargeFingerprintEntry())
			} else {
				add(chargeFingerprintEntry())
			}
		}
	}

	for key, value := range r.observationCursors {
		if txn != nil && txn.observationCursors != nil {
			if replacement, exists := txn.observationCursors[key]; exists {
				value = replacement
			}
		}
		if value != nil {
			add(chargeCursorEntry(key, *value))
		}
	}
	if txn != nil {
		for key, value := range txn.observationCursors {
			if _, exists := r.observationCursors[key]; !exists && value != nil {
				add(chargeCursorEntry(key, *value))
			}
		}
	}

	for key, value := range r.currentIncarnations {
		if txn != nil && txn.currentIncarnations != nil {
			if replacement, exists := txn.currentIncarnations[key]; exists {
				value = replacement
			}
		}
		if value != nil {
			add(chargeCurrentIncarnationEntry(key, *value))
		}
	}
	if txn != nil {
		for key, value := range txn.currentIncarnations {
			if _, exists := r.currentIncarnations[key]; !exists && value != nil {
				add(chargeCurrentIncarnationEntry(key, *value))
			}
		}
	}

	for key, value := range r.retiredIncarnations {
		if value != nil {
			add(chargeRetiredProofEntry(key, *value))
		}
	}
	if txn != nil {
		for key, value := range txn.retiredIncarnations {
			if _, exists := r.retiredIncarnations[key]; !exists && value != nil {
				add(chargeRetiredProofEntry(key, *value))
			}
		}
	}

	for key, value := range r.sequenceRecords {
		if txn != nil && txn.sequenceRecords != nil {
			if replacement, exists := txn.sequenceRecords[key]; exists {
				value = replacement
			}
		}
		add(chargeSequenceEntry(key, value))
	}
	if txn != nil {
		for key, value := range txn.sequenceRecords {
			if _, exists := r.sequenceRecords[key]; !exists {
				add(chargeSequenceEntry(key, value))
			}
		}
	}
	for key, value := range r.approvalRelationships {
		if txn != nil && txn.approvalRelationships != nil {
			if replacement, exists := txn.approvalRelationships[key]; exists {
				value = replacement
			}
		}
		add(chargeApprovalEntry(key, value))
	}
	if txn != nil {
		for key, value := range txn.approvalRelationships {
			if _, exists := r.approvalRelationships[key]; !exists {
				add(chargeApprovalEntry(key, value))
			}
		}
	}
	for key, value := range r.messageExpiryIndex {
		if txn != nil && txn.messageExpiryIndex != nil {
			if replacement, exists := txn.messageExpiryIndex[key]; exists {
				value = replacement
			}
		}
		add(chargeMessageExpiryEntry(key, value))
	}
	if txn != nil {
		for key, value := range txn.messageExpiryIndex {
			if _, exists := r.messageExpiryIndex[key]; !exists {
				add(chargeMessageExpiryEntry(key, value))
			}
		}
	}

	for key, value := range r.gaps {
		if txn != nil && txn.gaps != nil {
			if replacement, exists := txn.gaps[key]; exists {
				value = replacement
			}
		}
		if value != nil {
			add(chargeActiveGapEntry(key, *value))
		}
	}
	if txn != nil {
		for key, value := range txn.gaps {
			if _, exists := r.gaps[key]; !exists && value != nil {
				add(chargeActiveGapEntry(key, *value))
			}
		}
	}

	for actor, values := range r.transitions {
		if txn != nil && txn.transitions != nil {
			if replacement, exists := txn.transitions[actor]; exists {
				values = replacement
			}
		}
		add(chargeTransitionEntry(actor, values, r.config.TransitionLimit))
	}
	if txn != nil {
		for actor, values := range txn.transitions {
			if _, exists := r.transitions[actor]; !exists {
				add(chargeTransitionEntry(actor, values, r.config.TransitionLimit))
			}
		}
	}
	for key, value := range r.healthEpochs {
		if txn != nil && txn.healthEpochs != nil {
			if replacement, exists := txn.healthEpochs[key]; exists {
				value = replacement
			}
		}
		if value != nil {
			add(chargeHealthEpochEntry(key, *value))
		}
	}
	if txn != nil {
		for key, value := range txn.healthEpochs {
			if _, exists := r.healthEpochs[key]; !exists && value != nil {
				add(chargeHealthEpochEntry(key, *value))
			}
		}
	}
	return result
}

func chargePublishedProjection(r *Reconciler, txn *reconcileTxn) chargeResult {
	if r == nil || txn == nil {
		return saturatedCharge()
	}
	result := validCharge(64 + 5*16)

	nodeCount := 0
	visitNode := func(value *nodeRecord) {
		if value == nil {
			return
		}
		nodeCount++
		charge := chargeNodeRecord(value.value)
		if !charge.ok || charge.bytes < 512 {
			result = saturatedCharge()
			return
		}
		result = addCharges(result, validCharge(charge.bytes-512))
		if value.value.Transitions != nil {
			result = addCharges(result, logicalSlice(cap(value.value.Transitions), 96))
			for _, transition := range value.value.Transitions {
				result = addCharges(result, logicalString(len(transition.Source.ID)))
			}
		}
	}
	for key, value := range r.nodes {
		if replacement, exists := txn.nodes[key]; exists {
			value = replacement
		}
		visitNode(value)
	}
	for key, value := range txn.nodes {
		if _, exists := r.nodes[key]; !exists {
			visitNode(value)
		}
	}
	nodeSlice := logicalSlice(nodeCount, 512)
	result = addCharges(result, nodeSlice)

	edgeCount := 0
	visitEdge := func(value *edgeRecord) {
		if value == nil {
			return
		}
		edgeCount++
		charge := chargePublicEdge(value.value)
		if !charge.ok || charge.bytes < 384 {
			result = saturatedCharge()
			return
		}
		result = addCharges(result, validCharge(charge.bytes-384))
	}
	for key, value := range r.edges {
		if replacement, exists := txn.edges[key]; exists {
			value = replacement
		}
		visitEdge(value)
	}
	for key, value := range txn.edges {
		if _, exists := r.edges[key]; !exists {
			visitEdge(value)
		}
	}
	result = addCharges(result, logicalSlice(edgeCount, 384))

	gapCount := 0
	visitGap := func(value *Gap) {
		if value == nil {
			return
		}
		gapCount++
		charge := chargeGap(*value)
		if !charge.ok || charge.bytes < 128 {
			result = saturatedCharge()
			return
		}
		result = addCharges(result, validCharge(charge.bytes-128))
	}
	for key, value := range r.gaps {
		if replacement, exists := txn.gaps[key]; exists {
			value = replacement
		}
		visitGap(value)
	}
	for key, value := range txn.gaps {
		if _, exists := r.gaps[key]; !exists {
			visitGap(value)
		}
	}
	result = addCharges(result, logicalSlice(gapCount, 128))
	return result
}

func chargeHealthEpochEntry(key contributionKey, _ healthEpoch) chargeResult {
	return addCharges(validCharge(64), chargeContributionKey(key), validCharge(64+2*16))
}
