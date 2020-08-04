// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	cacheValidity time.Duration = 10 * time.Minute
)

var processReportedFields = []string{
	compliance.ProcessFieldName,
	compliance.ProcessFieldExe,
	compliance.ProcessFieldCmdLine,
}

func checkProcess(e env.Env, id string, res compliance.Resource) (*compliance.Report, error) {
	if res.Process == nil {
		return nil, fmt.Errorf("%s: expecting process resource in process check", id)
	}

	expr, err := eval.Cache.ParseIterable(res.Condition)
	if err != nil {
		return nil, err
	}

	process := res.Process

	log.Debugf("%s: running process check: %s", id, process.Name)

	processes, err := getProcesses(cacheValidity)

	if err != nil {
		return nil, log.Errorf("%s: Unable to fetch processes: %v", id, err)
	}

	matchedProcesses := processes.findProcessesByName(process.Name)

	var instances []*eval.Instance
	for _, mp := range matchedProcesses {

		flagValues := parseProcessCmdLine(mp.Cmdline)
		instance := &eval.Instance{
			Vars: eval.VarMap{
				compliance.ProcessFieldName:    mp.Name,
				compliance.ProcessFieldExe:     mp.Exe,
				compliance.ProcessFieldCmdLine: mp.Cmdline,
			},
			Functions: eval.FunctionMap{
				compliance.ProcessFuncFlag:    processFlag(flagValues),
				compliance.ProcessFuncHasFlag: processHasFlag(flagValues),
			},
		}
		instances = append(instances, instance)
	}

	it := &instanceIterator{
		instances: instances,
	}

	result, err := expr.EvaluateIterator(it, globalInstance)
	if err != nil {
		return nil, err
	}

	if res.Fallback != nil {
		fallbackExpr, err := eval.Cache.ParseExpression(res.Fallback.Condition)
		if err != nil {
			return nil, err
		}

		useFallback, err := fallbackExpr.BoolEvaluate(result.Instance)
		if err != nil {
			return nil, err
		}
		if useFallback {
			return nil, ErrResourceUseFallback
		}
	}

	return instanceResultToReport(result, processReportedFields), nil
}

func processFlag(flagValues map[string]string) eval.Function {
	return func(_ *eval.Instance, args ...interface{}) (interface{}, error) {
		flag, err := validateProcessFlagArg(args...)
		if err != nil {
			return nil, err
		}
		value, _ := flagValues[flag]
		return value, nil
	}
}
func processHasFlag(flagValues map[string]string) eval.Function {
	return func(_ *eval.Instance, args ...interface{}) (interface{}, error) {
		flag, err := validateProcessFlagArg(args...)
		if err != nil {
			return nil, err
		}
		_, has := flagValues[flag]
		return has, nil
	}
}

func validateProcessFlagArg(args ...interface{}) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf(`invalid number of arguments, expecting 1 got %d`, len(args))
	}
	flag, ok := args[0].(string)
	if !ok {
		return "", errors.New(`expecting string value for flag argument`)
	}
	return flag, nil
}
