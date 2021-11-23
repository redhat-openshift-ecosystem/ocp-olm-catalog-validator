// Copyright 2021 The OpenShift OLM Catalog Validator Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package validation

import (
	"encoding/json"
	golangerrors "errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/blang/semver"
	"github.com/operator-framework/api/pkg/validation"

	"github.com/operator-framework/api/pkg/manifests"
	"github.com/operator-framework/api/pkg/validation/errors"
	interfaces "github.com/operator-framework/api/pkg/validation/interfaces"
)

// FilePathKey defines the key which can be used by its consumers
// to inform where their index bundle image or annotations path to be checked
// (e.g. --optional-values="file==/path")
const FilePathKey = "file"

// RangeKey defines the key which can be used by its consumers
// to inform where their label range value only to be checked
// (e.g. --optional-values="range==v4.5-v4.8")
const RangeKey = "range"

// ocpLabel defines the OCP label which allow configure the OCP versions
// where the bundle will be distributed
const ocpLabel = "com.redhat.openshift.versions"

// deprecateOcpLabelMsg1_22 returns the specific ocp label message which is valid only for 1.22/OCP 4.9
const deprecateOcpLabelMsg1_22 = "this bundle is using APIs which were deprecated and " +
	"removed in v1.22. More info: https://kubernetes.io/docs/reference/using-api/deprecation-guide/#v1-22. " +
	"Migrate the APIs " +
	"for %s or provide compatible version(s) via the labels. (e.g. LABEL %s='4.6-4.8')"

// OCP version where the apis v1beta1 is no longer supported
const ocpVerV1beta1Unsupported = "4.9"

// OCP docs with the information to manage versions
const ocpDocLinkManagingVersions = "https://docs.openshift.com/container-platform/4.8/operators/operator_sdk/osdk-working-bundle-images.html#osdk-control-compat_osdk-working-bundle-images"

// Ensure that has the OCPMaxAnnotation
const olmproperties = "olm.properties"
const olmmaxOcpVersion = "olm.maxOpenShiftVersion"

// OpenShiftValidator validates the bundle manifests against the required criteria to publish
// the projects on the OpenShift catalog
//
// Note that this validator allows to receive a List of optional values as key=values:
// - file: expected the index bundle image(bundle.Dockerfile) or annotations path
// - range: expected an string value with the syntax described in https://redhat-connect.gitbook.io/certified-operator-guide/ocp-deployment/operator-metadata/bundle-directory/managing-openshift-versions
//
// Be aware that this validator is in alpha stage and can be changed. Also, the intention here is to decouple
// this validator and move it out of this project. Following its current checks:
//
// - Ensure that when found the usage of the removed APIs on 1.22/OCP 4.9 the CSV has the annotation
// olm.maxOpenShiftVersion with a value <= 4.8 and the OCP label com.redhat.openshift.versions with
// a value that does not contain OCP 4.9 or upper versions.
//
// - Ensure that the value informed in olm.maxOpenShiftVersion is compatible with the value informed
// via the com.redhat.openshift.versions label.
//
// - Ensure that the olm.maxOpenShiftVersion value respects semver
//
// - Ensure that the com.redhat.openshift.versions value respects semver
//
// Note the OCP label has been only be checked when the file is informed via the optional key values and with the file key. (Be aware
// that we might want to begin to check the metadata/annotations.yaml by default)
var OpenShiftValidator interfaces.Validator = interfaces.ValidatorFunc(openShiftValidator)

func openShiftValidator(objs ...interface{}) (results []errors.ManifestResult) {
	var filePath = ""
	var labelRange = ""
	for _, obj := range objs {
		switch obj := obj.(type) {
		case map[string]string:
			filePath = obj[FilePathKey]
			if len(filePath) > 0 {
				break
			}
			labelRange = obj[RangeKey]
			if len(labelRange) > 0 {
				break
			}
		}
	}

	for _, obj := range objs {
		switch v := obj.(type) {
		case *manifests.Bundle:
			results = append(results, validateOpenShiftBundle(v, filePath, labelRange))
		}
	}

	return results
}

// OpenShiftOperatorChecks defines the attributes used to perform the checks
type OpenShiftOperatorChecks struct {
	bundle           manifests.Bundle
	filePath         string
	labelRange       string
	rangeValue       string
	maxValue         string
	deprecateAPIsMsg string
	errs             []error
	warns            []error
}

// validateOpenShiftBundle will check the bundle against the criteria to publish into OpenShift Catalog
func validateOpenShiftBundle(bundle *manifests.Bundle, indexImagePath string, labelRange string) errors.ManifestResult {
	result := errors.ManifestResult{}
	if bundle == nil {
		result.Add(errors.ErrInvalidBundle("Bundle is nil", nil))
		return result
	}
	result.Name = bundle.Name

	if bundle.CSV == nil {
		result.Add(errors.ErrInvalidBundle("Bundle csv is nil", bundle.Name))
		return result
	}

	checks := OpenShiftOperatorChecks{bundle: *bundle, filePath: indexImagePath, labelRange: labelRange, rangeValue: labelRange, errs: []error{}, warns: []error{}}

	objs := bundle.ObjectsToValidate()
	for _, obj := range bundle.Objects {
		objs = append(objs, obj)
	}

	// pass the objects to the validator
	resultDeprecation := validation.AlphaDeprecatedAPIsValidator.Validate(objs...)

	for _, res := range resultDeprecation {
		for _, res := range res.Warnings {
			result.Add(errors.WarnFailedValidation(res.Detail, bundle.CSV.GetName()))
			checks.deprecateAPIsMsg = res.Detail
		}
	}

	checks = getMaxAnnotationValue(checks)
	checks = checkMaxVersionAnnotation(checks)
	checks = getOCPLabel(checks)
	checks = checkOCPLabel(checks)
	checks = validateOCPLabelWithMaxVersion(checks)
	for _, err := range checks.errs {
		result.Add(errors.ErrInvalidCSV(err.Error(), bundle.CSV.GetName()))
	}
	for _, warn := range checks.warns {
		result.Add(errors.WarnInvalidCSV(warn.Error(), bundle.CSV.GetName()))
	}

	return result
}

type propertiesAnnotation struct {
	Type  string
	Value string
}

// checkMaxVersionAnnotation will verify if the OpenShiftVersion property was informed
func getMaxAnnotationValue(checks OpenShiftOperatorChecks) OpenShiftOperatorChecks {

	properties := checks.bundle.CSV.Annotations[olmproperties]
	if len(properties) == 0 {
		return checks
	}

	var properList []propertiesAnnotation
	if err := json.Unmarshal([]byte(properties), &properList); err != nil {
		checks.errs = append(checks.errs, fmt.Errorf("csv.Annotations has an invalid value specified for %s. "+
			"Please, check the value (%s) and ensure that it is an array such as: "+
			"\"olm.properties\": '[{\"type\": \"key name\", \"value\": \"key value\"}]'",
			olmproperties, properties))
		return checks
	}

	for _, v := range properList {
		if v.Type == olmmaxOcpVersion {
			checks.maxValue = v.Value
			break
		}
	}

	return checks
}

// checkMaxVersionAnnotation will verify if the OpenShiftVersion property was informed
func checkMaxVersionAnnotation(checks OpenShiftOperatorChecks) OpenShiftOperatorChecks {
	if len(checks.deprecateAPIsMsg) > 0 && len(checks.maxValue) < 1 {
		checks.errs = append(checks.errs, fmt.Errorf("%s csv.Annotations not specified with an "+
			"OCP version lower than %s. This annotation is required to prevent the user from upgrading their OCP cluster "+
			"before they have installed a version of their operator which is compatible with %s. For further information see %s",
			olmmaxOcpVersion,
			ocpVerV1beta1Unsupported,
			ocpVerV1beta1Unsupported,
			ocpDocLinkManagingVersions))
		return checks
	}

	if len(checks.maxValue) > 0 {
		semVerVersionMaxOcp, err := semver.ParseTolerant(checks.maxValue)
		if err != nil {
			checks.errs = append(checks.errs, fmt.Errorf("csv.Annotations.%s has an invalid value. "+
				"Unable to parse (%s) using semver : %s",
				olmproperties, checks.maxValue, err))
			return checks
		}

		truncatedMaxOcp := semver.Version{Major: semVerVersionMaxOcp.Major, Minor: semVerVersionMaxOcp.Minor}
		if !semVerVersionMaxOcp.EQ(truncatedMaxOcp) {
			checks.warns = append(checks.warns, fmt.Errorf("csv.Annotations.%s has an invalid value. "+
				"%s must specify only major.minor versions, %s will be truncated to %s",
				olmproperties, olmmaxOcpVersion, semVerVersionMaxOcp, truncatedMaxOcp))
			return checks
		}

		if len(checks.deprecateAPIsMsg) > 0 {
			semVerOCPV1beta1Unsupported, _ := semver.ParseTolerant(ocpVerV1beta1Unsupported)
			if semVerVersionMaxOcp.GE(semVerOCPV1beta1Unsupported) {
				checks.errs = append(checks.errs, fmt.Errorf("invalid value for %s. "+
					"The OCP version value %s is >= of %s. Note that %s",
					olmmaxOcpVersion,
					checks.maxValue,
					ocpVerV1beta1Unsupported,
					checks.deprecateAPIsMsg))
				return checks
			}
		}
	}

	return checks
}

// checkOCPLabels will ensure that OCP labels are set and with a ocp targetVersion < 4.9
func getOCPLabel(checks OpenShiftOperatorChecks) OpenShiftOperatorChecks {
	if hasOCPLabelInfo(checks) {
		if len(checks.labelRange) > 0 {
			return checks
		}
		return getOCPLabelFromFile(checks)
	}
	return checks
}

// checkOCPLabels will ensure that OCP labels are set and with a ocp targetVersion < 4.9
func checkOCPLabel(checks OpenShiftOperatorChecks) OpenShiftOperatorChecks {
	// Note that we cannot make mandatory because the package format still valid
	if hasOCPLabelInfo(checks) && len(checks.rangeValue) == 0 {
		if len(checks.deprecateAPIsMsg) > 0 {
			checks.errs = append(checks.errs, fmt.Errorf(deprecateOcpLabelMsg1_22,
				checks.deprecateAPIsMsg,
				ocpLabel))
		} else {
			checks.warns = append(checks.warns, fmt.Errorf("unable to find %s configuration", ocpLabel))
		}
	}

	return checkOCPLabelFor4_9(checks)
}

func hasOCPLabelInfo(checks OpenShiftOperatorChecks) bool {
	return len(checks.filePath) != 0 || len(checks.labelRange) != 0
}

func getOCPLabelFromFile(checks OpenShiftOperatorChecks) OpenShiftOperatorChecks {
	if len(checks.filePath) > 0 {
		info, err := os.Stat(checks.filePath)
		if err != nil {
			checks.errs = append(checks.errs, fmt.Errorf("the file path informed (%s) was not found. "+
				"Error : %s", checks.filePath, err))
			return checks
		}
		if info.IsDir() {
			checks.errs = append(checks.errs, fmt.Errorf("the file path informed (%s) is not a file",
				checks.filePath))
			return checks
		}

		b, err := ioutil.ReadFile(checks.filePath)
		if err != nil {
			checks.errs = append(checks.errs, fmt.Errorf("unable to read the index image in the path "+
				"(%s). Error : %s", checks.filePath, err))
			return checks
		}

		indexPathContent := string(b)
		hasOCPLabel := strings.Contains(indexPathContent, ocpLabel)
		if hasOCPLabel {
			line := strings.Split(indexPathContent, "\n")
			for i := 0; i < len(line); i++ {
				if strings.Contains(line[i], ocpLabel) {
					if !strings.Contains(line[i], "=") && !strings.Contains(line[i], ":") {
						checks.errs = append(checks.errs, fmt.Errorf("invalid syntax (%s) for (%s)",
							line[i],
							ocpLabel))
						return checks
					}

					value := strings.Split(line[i], ocpLabel)
					if len(value[1]) == 0 {
						checks.errs = append(checks.errs, fmt.Errorf("invalid syntax (%s) for (%s)",
							line[i],
							ocpLabel))
						return checks
					}
					checks.rangeValue = cleanStringToGetTheVersionToParse(value[1])
					break
				}
			}
		}
	}
	return checks
}

func validateOCPLabelWithMaxVersion(checks OpenShiftOperatorChecks) OpenShiftOperatorChecks {
	if len(checks.maxValue) > 0 && len(checks.rangeValue) > 0 {
		isPartOfTarget, err := rangeContainsVersion(checks.rangeValue, cleanStringToGetTheVersionToParse(checks.maxValue), true)
		if err != nil {
			checks.errs = append(checks.errs, fmt.Errorf("error invalid label range %s",
				err))
			return checks
		}
		if !isPartOfTarget {
			checks.errs = append(checks.errs, fmt.Errorf("the %s annotation with the value %s to block the "+
				"cluster upgrade is incompatible with the versions where this solutions should be distributed "+
				"(%s with the value %s). For further information see %s",
				olmmaxOcpVersion,
				checks.maxValue,
				ocpLabel,
				checks.rangeValue,
				ocpDocLinkManagingVersions))
			return checks
		}
	}
	return checks
}

// todo: the ocp targetVersion version ought to be passed as parameter
// this code needs to be improved with the check for deprecated apis before/for 1.25
func checkOCPLabelFor4_9(checks OpenShiftOperatorChecks) OpenShiftOperatorChecks {
	if len(checks.deprecateAPIsMsg) > 0 && len(checks.rangeValue) > 0 {
		isPartOfTarget, err := rangeContainsVersion(checks.rangeValue, ocpVerV1beta1Unsupported, false)
		if err != nil {
			checks.errs = append(checks.errs, fmt.Errorf("error to validate the OpenShit label range: %s",
				err))
			return checks
		}
		if isPartOfTarget {
			checks.errs = append(checks.errs, fmt.Errorf("this bundle is using APIs which were "+
				"deprecated and removed in v1.22. "+
				"More info: https://kubernetes.io/docs/reference/using-api/deprecation-guide/#v1-22. "+
				"Migrate the API(s) for "+
				"%s or provide compatible version(s) by using the %s annotation in "+
				"`metadata/annotations.yaml` to ensure that the index image will be geneared "+
				"with its label. (e.g. LABEL %s='4.6-4.8')",
				checks.deprecateAPIsMsg,
				ocpLabel,
				ocpLabel))
		}
	}
	return checks
}

// rangeContainsVersion expected the range and the targetVersion version and returns true
// when the targetVersion version contains in the range
func rangeContainsVersion(r string, v string, tolerantParse bool) (bool, error) {
	if len(r) == 0 {
		return false, golangerrors.New("range is empty")
	}
	if len(v) == 0 {
		return false, golangerrors.New("version is empty")
	}

	v = strings.TrimPrefix(v, "v")
	compV, err := semver.Parse(v + ".0")
	if err != nil {
		splitTarget := strings.Split(v, ".")
		if tolerantParse {
			compV, err = semver.Parse(splitTarget[0] + "." + splitTarget[1] + ".0")
			if err != nil {
				return false, fmt.Errorf("invalid truncated version %q: %t", compV, err)
			}
		} else {
			return false, fmt.Errorf("invalid version %q: %t", v, err)
		}
	}

	// special legacy cases
	if r == "v4.5,v4.6" || r == "v4.6,v4.5" {
		semverRange := semver.MustParseRange(">=4.5.0")
		return semverRange(compV), nil
	}

	var semverRange semver.Range
	rs := strings.SplitN(r, "-", 2)
	switch len(rs) {
	case 1:
		// Range specify exact version
		if strings.HasPrefix(r, "=") {
			trimmed := strings.TrimPrefix(r, "=v")
			semverRange, err = semver.ParseRange(fmt.Sprintf("%s.0", trimmed))
		} else {
			trimmed := strings.TrimPrefix(r, "v")
			// Range specifies minimum version
			semverRange, err = semver.ParseRange(fmt.Sprintf(">=%s.0", trimmed))
		}
		if err != nil {
			return false, fmt.Errorf("invalid range %q: %v", r, err)
		}
	case 2:
		min := rs[0]
		max := rs[1]
		if strings.HasPrefix(min, "=") || strings.HasPrefix(max, "=") {
			return false, fmt.Errorf("invalid range %q: cannot use equal prefix with range", r)
		}
		min = strings.TrimPrefix(min, "v")
		max = strings.TrimPrefix(max, "v")
		semverRangeStr := fmt.Sprintf(">=%s.0 <=%s.0", min, max)
		semverRange, err = semver.ParseRange(semverRangeStr)
		if err != nil {
			return false, fmt.Errorf("invalid range %q: %v", r, err)
		}
	default:
		return false, fmt.Errorf("invalid range %q", r)
	}
	return semverRange(compV), nil
}

// cleanStringToGetTheVersionToParse will remove the expected characters for
// we are able to parse the version informed.
func cleanStringToGetTheVersionToParse(value string) string {
	// requires remove the possible double and single quotes which
	// are faced after read/parse the file.
	doubleQuote := "\""
	singleQuote := "'"
	value = strings.ReplaceAll(value, singleQuote, "")
	value = strings.ReplaceAll(value, doubleQuote, "")
	// requires remove = when the file informed is a index image
	value = strings.TrimPrefix(value, "=")
	// requires remove : and spaces when the file informed is annotation
	value = strings.TrimPrefix(value, ":")
	value = strings.TrimSpace(value)
	return value
}
