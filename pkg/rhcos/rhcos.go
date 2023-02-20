package rhcos

import (
	"encoding/json"
	"fmt"
	releasecontroller "github.com/openshift/release-controller/pkg/release-controller"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	changeoverTimestamp = 202212000000

	serviceScheme = "https"
	serviceUrl    = "releases-rhcos-art.apps.ocp-virt.prod.psi.redhat.com"

	reMdPromotedFrom = regexp.MustCompile("Promoted from (.*):(.*)")
	reMdRHCoSDiff    = regexp.MustCompile(`\* Red Hat Enterprise Linux CoreOS upgraded from ((\d)(\d+)\.[\w\.\-]+) to ((\d)(\d+)\.[\w\.\-]+)\n`)
	reMdRHCoSVersion = regexp.MustCompile(`\* Red Hat Enterprise Linux CoreOS ((\d)(\d+)\.[\w\.\-]+)\n`)

	reJsonRHCoSVersion2 = regexp.MustCompile(`((\d)(\d+))\.(\d+)\.(\d+)-(\d+)`)
)

func TransformMarkDownOutput(markdown, fromTag, toTag, architecture, architectureExtension string) (string, error) {
	// replace references to the previous version with links
	rePrevious, err := regexp.Compile(fmt.Sprintf(`([^\w:])%s(\W)`, regexp.QuoteMeta(fromTag)))
	if err != nil {
		return "", err
	}

	// do a best effort replacement to change out the headers
	markdown = strings.Replace(markdown, fmt.Sprintf(`# %s`, toTag), "", -1)
	if changed := strings.Replace(markdown, fmt.Sprintf(`## Changes from %s`, fromTag), "", -1); len(changed) != len(markdown) {
		markdown = fmt.Sprintf("## Changes from %s\n%s", fromTag, changed)
	}
	markdown = rePrevious.ReplaceAllString(markdown, fmt.Sprintf("$1[%s](/releasetag/%s)$2", fromTag, fromTag))

	// add link to tag from which current version promoted from
	markdown = reMdPromotedFrom.ReplaceAllString(markdown, fmt.Sprintf("Release %s was created from [$1:$2](/releasetag/$2)", toTag))

	// TODO: As we get more comfortable with these sorts of transformations, we could make them more generic.
	//       For now, this will have to do.
	if m := reMdRHCoSDiff.FindStringSubmatch(markdown); m != nil {
		var ok bool
		var fromURL, toURL url.URL
		var fromStream, toStream string

		fromRelease := m[1]
		if fromStream, ok = getRHCoSReleaseStream(fromRelease, architectureExtension); ok {
			fromURL = url.URL{
				Scheme:   serviceScheme,
				Host:     serviceUrl,
				Path:     "/",
				Fragment: fromRelease,
				RawQuery: (url.Values{
					"stream":  []string{fromStream},
					"arch":    []string{architecture},
					"release": []string{fromRelease},
				}).Encode(),
			}
		}

		toRelease := m[4]
		if toStream, ok = getRHCoSReleaseStream(toRelease, architectureExtension); ok {
			toURL = url.URL{
				Scheme:   serviceScheme,
				Host:     serviceUrl,
				Path:     "/",
				Fragment: toRelease,
				RawQuery: (url.Values{
					"stream":  []string{toStream},
					"arch":    []string{architecture},
					"release": []string{toRelease},
				}).Encode(),
			}
		}
		diffURL := url.URL{
			Scheme: serviceScheme,
			Host:   serviceUrl,
			Path:   "/diff.html",
			RawQuery: (url.Values{
				"first_stream":   []string{fromStream},
				"first_release":  []string{fromRelease},
				"second_stream":  []string{toStream},
				"second_release": []string{toRelease},
				"arch":           []string{architecture},
			}).Encode(),
		}
		replace := fmt.Sprintf(
			`* Red Hat Enterprise Linux CoreOS upgraded from [%s](%s) to [%s](%s) ([diff](%s))`+"\n",
			fromRelease,
			fromURL.String(),
			toRelease,
			toURL.String(),
			diffURL.String(),
		)
		markdown = strings.ReplaceAll(markdown, m[0], replace)
	}
	if m := reMdRHCoSVersion.FindStringSubmatch(markdown); m != nil {
		var ok bool
		var fromURL url.URL
		var fromStream string

		fromRelease := m[1]
		if fromStream, ok = getRHCoSReleaseStream(fromRelease, architectureExtension); ok {
			fromURL = url.URL{
				Scheme:   serviceScheme,
				Host:     serviceUrl,
				Path:     "/",
				Fragment: fromRelease,
				RawQuery: (url.Values{
					"stream":  []string{fromStream},
					"arch":    []string{architecture},
					"release": []string{fromRelease},
				}).Encode(),
			}
		}
		replace := fmt.Sprintf(
			`* Red Hat Enterprise Linux CoreOS [%s](%s)`+"\n",
			fromRelease,
			fromURL.String(),
		)
		markdown = strings.ReplaceAll(markdown, m[0], replace)
	}
	return markdown, nil
}

func TransformJsonOutput(output, architecture, architectureExtension string) (string, error) {
	var changeLogJson releasecontroller.ChangeLog
	err := json.Unmarshal([]byte(output), &changeLogJson)
	if err != nil {
		return "", err
	}

	for i, component := range changeLogJson.Components {
		switch component.Name {
		case "Red Hat Enterprise Linux CoreOS":
			var ok bool
			var fromStream, toStream string
			if len(component.Version) == 0 {
				continue
			}
			if toStream, ok = getRHCoSReleaseStream(component.Version, architectureExtension); ok {
				toURL := url.URL{
					Scheme:   serviceScheme,
					Host:     serviceUrl,
					Path:     "/",
					Fragment: component.Version,
					RawQuery: (url.Values{
						"stream":  []string{toStream},
						"arch":    []string{architecture},
						"release": []string{component.Version},
					}).Encode(),
				}
				component.VersionUrl = toURL.String()
			}

			if len(component.From) > 0 {
				if fromStream, ok = getRHCoSReleaseStream(component.From, architectureExtension); ok {
					fromUrl := url.URL{
						Scheme:   serviceScheme,
						Host:     serviceUrl,
						Path:     "/",
						Fragment: component.From,
						RawQuery: (url.Values{
							"stream":  []string{fromStream},
							"arch":    []string{architecture},
							"release": []string{component.From},
						}).Encode(),
					}
					component.FromUrl = fromUrl.String()

					diffURL := url.URL{
						Scheme: serviceScheme,
						Host:   serviceUrl,
						Path:   "/diff.html",
						RawQuery: (url.Values{
							"first_stream":   []string{fromStream},
							"first_release":  []string{component.From},
							"second_stream":  []string{toStream},
							"second_release": []string{component.Version},
							"arch":           []string{architecture},
						}).Encode(),
					}
					component.DiffUrl = diffURL.String()
				}
			}
			changeLogJson.Components[i] = component
		}
	}

	updated, err := json.MarshalIndent(&changeLogJson, "", "  ")
	if err != nil {
		return "", err
	}

	return string(updated), nil
}

func getRHCoSReleaseStream(version, architectureExtension string) (string, bool) {
	if m := reJsonRHCoSVersion2.FindStringSubmatch(version); m != nil {
		ts, err := strconv.Atoi(m[5])
		if err != nil {
			return "", false
		}
		minor, err := strconv.Atoi(m[3])
		if err != nil {
			return "", false
		}
		switch {
		case ts > changeoverTimestamp && minor >= 9:
			return fmt.Sprintf("prod/streams/%s.%s", m[2], m[3]), true
		default:
			return fmt.Sprintf("releases/rhcos-%s.%s%s", m[2], m[3], architectureExtension), true
		}
	}
	return "", false
}
