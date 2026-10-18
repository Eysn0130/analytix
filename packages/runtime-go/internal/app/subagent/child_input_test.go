package subagent

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func childInputContractArgsV1() map[string]any {
	return map[string]any{
		"mode": "start_or_steer", "threadId": "thread_1", "jobId": "job-1",
		"expectedRunId": "job-1", "expectedChildTurnId": "turn_1", "message": "Continue this work",
		"clientMessageId": "11111111-1111-4111-8111-111111111111",
	}
}

func TestChildInputRequestV1ClosedIntent(t *testing.T) {
	request, err := ChildInputRequestFromArgsV1(childInputContractArgsV1())
	if err != nil || request.JobID != "job-1" || request.Mode != ChildInputStartOrSteerV1 {
		t.Fatalf("request = %#v, error = %v", request, err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"alias":            func(args map[string]any) { args["job_id"] = "job-1" },
		"wrong type":       func(args map[string]any) { args["expectedRunId"] = 1 },
		"missing run":      func(args map[string]any) { delete(args, "expectedRunId") },
		"missing turn":     func(args map[string]any) { delete(args, "expectedChildTurnId") },
		"nonopaque client": func(args map[string]any) { args["clientMessageId"] = "item_turn_1_user" },
		"binding":          func(args map[string]any) { args["binding"] = "forged" },
		"origin":           func(args map[string]any) { args["origin"] = "host" },
		"model source":     func(args map[string]any) { args["sourceToolCallId"] = "call_1" },
		"grant":            func(args map[string]any) { args["executionGrant"] = "forged" },
		"long message":     func(args map[string]any) { args["message"] = strings.Repeat("界", DefaultTaskJobSteerMaxRunes+1) },
	} {
		t.Run(name, func(t *testing.T) {
			args := childInputContractArgsV1()
			mutate(args)
			if _, err := ChildInputRequestFromArgsV1(args); err == nil {
				t.Fatal("invalid intent accepted")
			}
		})
	}
}

func TestChildInputRequestV1InspectIsReadOnly(t *testing.T) {
	args := map[string]any{"mode": "inspect", "threadId": "thread_1", "jobId": "job-1"}
	if _, err := ChildInputRequestFromArgsV1(args); err != nil {
		t.Fatal(err)
	}
	args["message"] = "do work"
	if _, err := ChildInputRequestFromArgsV1(args); err == nil {
		t.Fatal("inspection accepted input")
	}
}

func TestChildInputDigestV1BindsInputAndLineage(t *testing.T) {
	request, err := ChildInputRequestFromArgsV1(childInputContractArgsV1())
	if err != nil {
		t.Fatal(err)
	}
	digest := ChildInputDigestV1(request)
	if digest == "" || digest != ChildInputDigestV1(request) {
		t.Fatal("input identity is unstable")
	}
	for name, mutate := range map[string]func(*ChildInputRequestV1){
		"parent":  func(r *ChildInputRequestV1) { r.ParentThreadID = "thread_2" },
		"source":  func(r *ChildInputRequestV1) { r.JobID = "job-2" },
		"run":     func(r *ChildInputRequestV1) { r.ExpectedRunID = "job-2" },
		"turn":    func(r *ChildInputRequestV1) { r.ExpectedChildTurnID = "turn_2" },
		"message": func(r *ChildInputRequestV1) { r.Message += " again" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := request
			mutate(&changed)
			if ChildInputDigestV1(changed) == digest {
				t.Fatal("changed input or lineage retained identity")
			}
		})
	}
}

func TestChildInputResponseV1RejectsUnconfirmedAndMalformedCombinations(t *testing.T) {
	base := func() map[string]any {
		return map[string]any{
			"schemaVersion": 1,
			"capabilities":  map[string]any{"send": "start", "stop": false, "runId": "job-1", "childThreadId": "child-1", "childTurnId": "turn_1"},
			"receipt":       map[string]any{"action": "start", "admission": "admitted", "clientMessageId": "11111111-1111-4111-8111-111111111111", "jobId": "job-2", "childThreadId": "child-1", "childTurnId": "turn_2", "messageId": "item_2"},
		}
	}
	if _, err := DecodeChildInputResponseV1(base()); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"start queued":         func(b map[string]any) { b["receipt"].(map[string]any)["admission"] = "queued" },
		"unknown with receipt": func(b map[string]any) { b["code"] = "child_input_unknown" },
		"wrong session":        func(b map[string]any) { b["receipt"].(map[string]any)["childThreadId"] = "child-2" },
		"partial observed ids": func(b map[string]any) {
			b["capabilities"].(map[string]any)["send"] = "disabled"
			delete(b["capabilities"].(map[string]any), "runId")
		},
		"missing stop":        func(b map[string]any) { delete(b["capabilities"].(map[string]any), "stop") },
		"null stop":           func(b map[string]any) { b["capabilities"].(map[string]any)["stop"] = nil },
		"start while stop":    func(b map[string]any) { b["capabilities"].(map[string]any)["stop"] = true },
		"steer without owner": func(b map[string]any) { b["capabilities"].(map[string]any)["send"] = "steer" },
		"null receipt":        func(b map[string]any) { b["receipt"] = nil },
		"authority":           func(b map[string]any) { b["authority"] = "forged" },
	} {
		t.Run(name, func(t *testing.T) {
			b := base()
			mutate(b)
			if _, err := DecodeChildInputResponseV1(b); err == nil {
				t.Fatal("invalid combination accepted")
			}
		})
	}
	queued := base()
	queued["capabilities"].(map[string]any)["send"] = "steer"
	queued["capabilities"].(map[string]any)["stop"] = true
	queued["receipt"].(map[string]any)["action"] = "steer"
	queued["receipt"].(map[string]any)["admission"] = "queued"
	if _, err := DecodeChildInputResponseV1(queued); err != nil {
		t.Fatal(err)
	}
}

func TestChildInputReceiptRequiresSuccessfulExistingStartOwnerV1(t *testing.T) {
	failed := NewBackgroundJobStartBarrier()
	if err := failed.StartIfActive(context.Background(), func() error { return errors.New("synthetic failed claim") }); err == nil || failed.StartedV1() {
		t.Fatal("failed callback reported real owner")
	}
	stopped := NewBackgroundJobStartBarrier()
	stopped.Cancel()
	if err := stopped.StartIfActive(context.Background(), func() error { return nil }); err == nil || stopped.StartedV1() {
		t.Fatal("cancelled callback reported real owner")
	}
	state := NewRuntimeState()
	barrier, registered := state.RegisterBackgroundJobWithStartBarrier("host-1", func() {})
	if !registered || state.BackgroundJobStartedV1("host-1") {
		t.Fatal("registration alone reported real owner")
	}
	defer state.UnregisterBackgroundJob("host-1")
	if err := barrier.StartIfActive(context.Background(), func() error { return nil }); err != nil || !state.BackgroundJobStartedV1("host-1") {
		t.Fatal("real start callback lost owner proof", err)
	}
}
