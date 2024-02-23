package statsig

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/statsig-io/ip3country-go/pkg/countrylookup"
	"github.com/ua-parser/uap-go/uaparser"
)

type evaluator struct {
	store         *store
	countryLookup *countrylookup.CountryLookup
	uaParser      *uaparser.Parser
}

type evalResult struct {
	Pass               bool
	ConfigValue        DynamicConfig
	FetchFromServer    bool
	Id                 string
	SecondaryExposures []map[string]string
}

const dynamicConfigType = "dynamic_config"

func newEvaluator(transport *transport) *evaluator {
	store := newStore(transport)
	parser := uaparser.NewFromSaved()
	countryLookup := countrylookup.New()
	defer func() {
		if err := recover(); err != nil {
			// TODO: log here
			fmt.Println(err)
		}
	}()

	return &evaluator{
		store:         store,
		countryLookup: countryLookup,
		uaParser:      parser,
	}
}

func (e *evaluator) Stop() {
	e.store.StopPolling()
}

func (e *evaluator) CheckGate(user User, gateName string) *evalResult {
	if gate, hasGate := e.store.featureGates[gateName]; hasGate {
		return e.eval(user, gate)
	}
	return new(evalResult)
}

func (e *evaluator) GetConfig(user User, configName string) *evalResult {
	if config, hasConfig := e.store.dynamicConfigs[configName]; hasConfig {
		return e.eval(user, config)
	}
	return new(evalResult)
}

func (e *evaluator) eval(user User, spec configSpec) *evalResult {
	var configValue map[string]interface{}
	isDynamicConfig := strings.ToLower(spec.Type) == dynamicConfigType
	if isDynamicConfig {
		err := json.Unmarshal(spec.DefaultValue, &configValue)
		if err != nil {
			configValue = make(map[string]interface{})
		}
	}

	var exposures []map[string]string
	defaultRuleID := "default"
	if spec.Enabled {
		for _, rule := range spec.Rules {
			r := e.evalRule(user, rule)
			if r.FetchFromServer {
				return r
			}
			exposures = append(exposures, r.SecondaryExposures...)
			if r.Pass {
				pass := evalPassPercent(user, rule, spec)
				if isDynamicConfig {
					if pass {
						var ruleConfigValue map[string]interface{}
						err := json.Unmarshal(rule.ReturnValue, &ruleConfigValue)
						if err != nil {
							ruleConfigValue = make(map[string]interface{})
						}
						configValue = ruleConfigValue
					}
					return &evalResult{
						Pass:               pass,
						ConfigValue:        *NewConfig(spec.Name, configValue, rule.ID),
						Id:                 rule.ID,
						SecondaryExposures: exposures}
				} else {
					return &evalResult{Pass: pass, Id: rule.ID, SecondaryExposures: exposures}
				}
			}
		}
	} else {
		defaultRuleID = "disabled"
	}

	if isDynamicConfig {
		return &evalResult{
			Pass:               false,
			ConfigValue:        *NewConfig(spec.Name, configValue, defaultRuleID),
			Id:                 defaultRuleID,
			SecondaryExposures: exposures}
	}
	return &evalResult{Pass: false, Id: defaultRuleID, SecondaryExposures: exposures}
}

func evalPassPercent(user User, rule configRule, spec configSpec) bool {
	ruleSalt := rule.Salt
	if ruleSalt == "" {
		ruleSalt = rule.ID
	}
	hash := getHash(spec.Salt + "." + ruleSalt + "." + getUnitID(user, rule.IDType))

	return hash%10000 < (uint64(rule.PassPercentage) * 100)
}

func getUnitID(user User, idType string) string {
	if idType != "" && strings.ToLower(idType) != "userid" {
		if val, ok := user.CustomIDs[idType]; ok {
			return val
		}
		if val, ok := user.CustomIDs[strings.ToLower(idType)]; ok {
			return val
		}
		return ""
	}
	return user.UserID
}

func (e *evaluator) evalRule(user User, rule configRule) *evalResult {
	var exposures []map[string]string
	var finalResult = &evalResult{Pass: true, FetchFromServer: false}
	for _, cond := range rule.Conditions {
		res := e.evalCondition(user, cond)
		if !res.Pass {
			finalResult.Pass = false
		}
		if res.FetchFromServer {
			finalResult.FetchFromServer = true
		}
		exposures = append(exposures, res.SecondaryExposures...)
	}
	finalResult.SecondaryExposures = exposures
	return finalResult
}

func (e *evaluator) evalCondition(user User, cond configCondition) *evalResult {
	var value interface{}
	switch cond.Type {
	case "public":
		return &evalResult{Pass: true}
	case "fail_gate", "pass_gate":
		dependentGateName, ok := cond.TargetValue.(string)
		if !ok {
			return &evalResult{Pass: false}
		}
		result := e.CheckGate(user, dependentGateName)
		if result.FetchFromServer {
			return &evalResult{FetchFromServer: true}
		}
		newExposure := map[string]string{
			"gate":      dependentGateName,
			"gateValue": strconv.FormatBool(result.Pass),
			"ruleID":    result.Id,
		}
		allExposures := append(result.SecondaryExposures, newExposure)
		if cond.Type == "pass_gate" {
			return &evalResult{Pass: result.Pass, SecondaryExposures: allExposures}
		} else {
			return &evalResult{Pass: !result.Pass, SecondaryExposures: allExposures}
		}
	case "ip_based":
		value = getFromUser(user, cond.Field)
		if value == nil || value == "" {
			value = getFromIP(user, cond.Field, e.countryLookup)
		}
	case "ua_based":
		value = getFromUser(user, cond.Field)
		if value == nil || value == "" {
			value = getFromUserAgent(user, cond.Field, e.uaParser)
		}
	case "user_field":
		value = getFromUser(user, cond.Field)
	case "environment_field":
		value = getFromEnvironment(user, cond.Field)
	case "current_time":
		value = time.Now().Unix() // time in seconds
	case "user_bucket":
		if salt, ok := cond.AdditionalValues["salt"]; ok {
			value = int64(getHash(fmt.Sprintf("%s.%s", salt, getUnitID(user, cond.IDType))) % 1000)
		}
	case "unit_id":
		value = getUnitID(user, cond.IDType)
	default:
		return &evalResult{FetchFromServer: true}
	}

	pass := false
	server := false
	switch cond.Operator {
	case "gt":
		pass = compareNumbers(value, cond.TargetValue, func(x, y float64) bool { return x > y })
	case "gte":
		pass = compareNumbers(value, cond.TargetValue, func(x, y float64) bool { return x >= y })
	case "lt":
		pass = compareNumbers(value, cond.TargetValue, func(x, y float64) bool { return x < y })
	case "lte":
		pass = compareNumbers(value, cond.TargetValue, func(x, y float64) bool { return x <= y })
	case "version_gt":
		pass = compareVersions(value, cond.TargetValue, func(x, y string) bool { return compareVersionsHelper(x, y) > 0 })
	case "version_gte":
		pass = compareVersions(value, cond.TargetValue, func(x, y string) bool { return compareVersionsHelper(x, y) >= 0 })
	case "version_lt":
		pass = compareVersions(value, cond.TargetValue, func(x, y string) bool { return compareVersionsHelper(x, y) < 0 })
	case "version_lte":
		pass = compareVersions(value, cond.TargetValue, func(x, y string) bool { return compareVersionsHelper(x, y) <= 0 })
	case "version_eq":
		pass = compareVersions(value, cond.TargetValue, func(x, y string) bool { return compareVersionsHelper(x, y) == 0 })
	case "version_neq":
		pass = compareVersions(value, cond.TargetValue, func(x, y string) bool { return compareVersionsHelper(x, y) != 0 })

	// array operations
	case "any":
		pass = arrayAny(cond.TargetValue, value, func(x, y interface{}) bool {
			return compareStrings(x, y, true, func(s1, s2 string) bool { return s1 == s2 })
		})
	case "none":
		pass = !arrayAny(cond.TargetValue, value, func(x, y interface{}) bool {
			return compareStrings(x, y, true, func(s1, s2 string) bool { return s1 == s2 })
		})
	case "any_case_sensitive":
		pass = arrayAny(cond.TargetValue, value, func(x, y interface{}) bool {
			return compareStrings(x, y, false, func(s1, s2 string) bool { return s1 == s2 })
		})
	case "none_case_sensitive":
		pass = !arrayAny(cond.TargetValue, value, func(x, y interface{}) bool {
			return compareStrings(x, y, false, func(s1, s2 string) bool { return s1 == s2 })
		})

	// string operations
	case "str_starts_with_any":
		pass = arrayAny(cond.TargetValue, value, func(x, y interface{}) bool {
			return compareStrings(x, y, true, func(s1, s2 string) bool { return strings.HasPrefix(s1, s2) })
		})
	case "str_ends_with_any":
		pass = arrayAny(cond.TargetValue, value, func(x, y interface{}) bool {
			return compareStrings(x, y, true, func(s1, s2 string) bool { return strings.HasSuffix(s1, s2) })
		})
	case "str_contains_any":
		pass = arrayAny(cond.TargetValue, value, func(x, y interface{}) bool {
			return compareStrings(x, y, true, func(s1, s2 string) bool { return strings.Contains(s1, s2) })
		})
	case "str_contains_none":
		pass = !arrayAny(cond.TargetValue, value, func(x, y interface{}) bool {
			return compareStrings(x, y, true, func(s1, s2 string) bool { return strings.Contains(s1, s2) })
		})
	case "str_matches":
		matched, _ := regexp.MatchString(cond.TargetValue.(string), value.(string))
		pass = matched

	// strict equality
	case "eq":
		pass = reflect.DeepEqual(value, cond.TargetValue)
	case "neq":
		pass = !reflect.DeepEqual(value, cond.TargetValue)

	// time
	case "before":
		pass = getTime(value).Before(getTime(cond.TargetValue))
	case "after":
		pass = getTime(value).After(getTime(cond.TargetValue))
	case "on":
		y1, m1, d1 := getTime(value).Date()
		y2, m2, d2 := getTime(cond.TargetValue).Date()
		pass = (y1 == y2 && m1 == m2 && d1 == d2)
	default:
		pass = false
		server = true
	}
	return &evalResult{Pass: pass, FetchFromServer: server}
}

func getFromUser(user User, field string) interface{} {
	var value interface{}
	// 1. Try to get from top level user field first
	switch strings.ToLower(field) {
	case "userid", "user_id":
		value = user.UserID
	case "email":
		value = user.Email
	case "ip", "ipaddress", "ip_address":
		value = user.IpAddress
	case "useragent", "user_agent":
		value = user.UserAgent
	case "country":
		value = user.Country
	case "locale":
		value = user.Locale
	case "appversion", "app_version":
		value = user.AppVersion
	}

	// 2. Check custom user attributes and then private attributes next
	if value == "" || value == nil {
		if customValue, ok := user.Custom[field]; ok {
			value = customValue
		} else if customValue, ok := user.Custom[strings.ToLower(field)]; ok {
			value = customValue
		} else if privateValue, ok := user.PrivateAttributes[field]; ok {
			value = privateValue
		} else if privateValue, ok := user.PrivateAttributes[strings.ToLower(field)]; ok {
			value = privateValue
		}
	}

	return value
}

func getFromEnvironment(user User, field string) string {
	var value string
	if val, ok := user.StatsigEnvironment[field]; ok {
		value = val
	}
	if val, ok := user.StatsigEnvironment[strings.ToLower(field)]; ok {
		value = val
	}
	return value
}

func getFromUserAgent(user User, field string, parser *uaparser.Parser) string {
	ua := getFromUser(user, "useragent")
	uaStr, ok := ua.(string)
	if !ok {
		return ""
	}
	client := parser.Parse(uaStr)
	switch strings.ToLower(field) {
	case "os_name", "osname":
		return client.Os.Family
	case "os_version", "osversion":
		return strings.Join(removeEmptyStrings([]string{client.Os.Major, client.Os.Minor, client.Os.Patch, client.Os.PatchMinor}), ".")
	case "browser_name", "browsername":
		return client.UserAgent.Family
	case "browser_version", "browserversion":
		return strings.Join(removeEmptyStrings([]string{client.UserAgent.Major, client.UserAgent.Minor, client.UserAgent.Patch}), ".")
	}
	return ""
}

func getFromIP(user User, field string, lookup *countrylookup.CountryLookup) string {
	if strings.ToLower(field) != "country" {
		return ""
	}

	ip := getFromUser(user, "ip")
	if ipStr, ok := ip.(string); ok {
		if res, lookupOK := lookup.LookupIp(ipStr); lookupOK {
			return res
		}
	}

	return ""
}

func removeEmptyStrings(s []string) []string {
	var r []string
	for _, str := range s {
		if str != "" {
			r = append(r, str)
		}
	}
	return r
}

func getHash(key string) uint64 {
	hasher := sha256.New()
	bytes := []byte(key)
	hasher.Write(bytes)
	return binary.BigEndian.Uint64(hasher.Sum(nil))
}

func getNumericValue(a interface{}) (float64, bool) {
	switch a.(type) {
	case int:
		return float64(a.(int)), true
	case int32:
		return float64(a.(int32)), true
	case int64:
		return float64(a.(int64)), true
	case uint64:
		return float64(a.(uint64)), true
	case float32:
		return float64(a.(float32)), true
	case float64:
		return a.(float64), true
	case string:
		s := string(a.(string))
		f, err := strconv.ParseFloat(s, 64)
		if err == nil {
			return f, true
		}
	}
	return 0, false
}

func compareNumbers(a, b interface{}, fun func(x, y float64) bool) bool {
	numA, okA := getNumericValue(a)
	numB, okB := getNumericValue(b)
	if !okA || !okB {
		return false
	}
	return fun(numA, numB)
}

func compareStrings(s1 interface{}, s2 interface{}, ignoreCase bool, fun func(x, y string) bool) bool {
	var str1, str2 string
	if s1 == nil || s2 == nil {
		return false
	}
	if reflect.TypeOf(s1).Kind() == reflect.String {
		str1 = s1.(string)
	} else {
		str1 = fmt.Sprintf("%v", s1)
	}
	if reflect.TypeOf(s2).Kind() == reflect.String {
		str2 = s2.(string)
	} else {
		str2 = fmt.Sprintf("%v", s2)
	}

	if ignoreCase {
		return fun(strings.ToLower(str1), strings.ToLower(str2))
	}
	return fun(str1, str2)
}

func compareVersionsHelper(v1 string, v2 string) int {
	i := 0
	v1Parts := strings.Split(v1, ".")
	v1len := len(v1Parts)
	v2Parts := strings.Split(v2, ".")
	v2len := len(v2Parts)
	for i < maxInt(v1len, v2len) {
		var p1 string
		if i >= v1len {
			p1 = "0"
		} else {
			p1 = v1Parts[i]
		}
		var p2 string
		if i >= v2len {
			p2 = "0"
		} else {
			p2 = v2Parts[i]
		}

		n1, _ := strconv.ParseInt(p1, 10, 64)
		n2, _ := strconv.ParseInt(p2, 10, 64)
		if n1 < n2 {
			return -1
		}
		if n1 > n2 {
			return 1
		}
		i++
	}
	return 0
}

func compareVersions(a, b interface{}, fun func(x, y string) bool) bool {
	strA, okA := a.(string)
	strB, okB := b.(string)
	if !okA || !okB {
		return false
	}
	v1 := strings.Split(strA, "-")[0]
	v2 := strings.Split(strB, "-")[0]
	if len(v1) == 0 || len(v2) == 0 {
		return false
	}
	return fun(v1, v2)
}

func maxInt(x, y int) int {
	if x > y {
		return x
	}
	return y
}

func arrayAny(arr interface{}, val interface{}, fun func(x, y interface{}) bool) bool {
	if array, ok := arr.([]interface{}); ok {
		for _, arrVal := range array {
			if fun(val, arrVal) {
				return true
			}
		}
	}
	return false
}

func getTime(a interface{}) time.Time {
	var t_sec time.Time
	var t_msec time.Time
	switch a.(type) {
	case float64:
		t_sec = time.Unix(int64(a.(float64)), 0)
		t_msec = time.Unix(int64(a.(float64))/1000, 0)
	case int64:
		t_sec = time.Unix(a.(int64), 0)
		t_msec = time.Unix(a.(int64)/1000, 0)
	case int32:
		t_sec = time.Unix(int64(a.(int32)), 0)
		t_msec = time.Unix(int64(a.(int32))/1000, 0)
	case int:
		t_sec = time.Unix(int64(a.(int)), 0)
		t_msec = time.Unix(int64(a.(int))/1000, 0)
	case string:
		v, err := strconv.ParseInt(a.(string), 10, 64)
		if err != nil {
			t_sec = time.Unix(v, 0)
			t_msec = time.Unix(v/1000, 0)
		}
	}
	if t_sec.Year() > time.Now().Year()+100 {
		return t_msec
	}
	return t_sec
}
