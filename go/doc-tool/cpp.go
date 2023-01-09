package doctool

/*
 * Copyright 2017 Yuji Ito <llamerada.jp@gmail.com>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
)

const HEAD = `
---
title: API for C++
type: docs
---
`

func Process(src, dst string) error {
	lines, err := read(src)
	if err != nil {
		return err
	}

	lines = deleteSummary(lines)
	lines = editHeading(lines)
	lines = skipEmtpyLine(lines)

	err = write(dst, HEAD, lines)
	if err != nil {
		return err
	}

	return nil
}

func read(src string) ([]string, error) {
	fp, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	defer fp.Close()

	scanner := bufio.NewScanner(fp)
	lines := make([]string, 0)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	err = scanner.Err()
	if err != nil {
		return nil, err
	}

	return lines, nil
}

func deleteSummary(lines []string) []string {
	expSummary := regexp.MustCompile("^#+ Summary$")
	expMembers := regexp.MustCompile("^#+ Members$")

	ret := make([]string, 0)
	inSummary := false

	for _, line := range lines {
		if expSummary.MatchString(line) {
			inSummary = true
			continue
		}

		if expMembers.MatchString(line) {
			inSummary = false
			continue
		}

		if !inSummary {
			ret = append(ret, line)
		}
	}

	return ret
}

func editHeading(lines []string) []string {
	expHeading := regexp.MustCompile("^(#+)[ ]+(.*)$")
	expDeleted := regexp.MustCompile("= delete")
	expBkQuote := regexp.MustCompile("`")
	expLink := regexp.MustCompile(`\[(.*?)\]\(#[a-zA-Z0-9_]*\)`)
	expPrefix := regexp.MustCompile("(public|explicit)[ ]+")

	ret := make([]string, 0)

	for _, line := range lines {
		matched := expHeading.FindStringSubmatch(line)
		if matched == nil {
			ret = append(ret, line)
			continue
		}

		signLen := len(matched[1])
		str := matched[2]

		if expDeleted.MatchString(str) {
			continue
		}

		if signLen == 1 {
			signLen = 2
		}
		if signLen == 4 && str != "Parameters" && str != "Returns" {
			signLen = 3
		}

		str = expBkQuote.ReplaceAllString(str, "")
		str = expPrefix.ReplaceAllString(str, "")
		str = expLink.ReplaceAllString(str, "$1")

		sign := ""
		for i := 0; i < signLen; i++ {
			sign = sign + "#"
		}

		ret = append(ret, sign+" "+str)
	}

	return ret
}

func skipEmtpyLine(lines []string) []string {
	expListed := regexp.MustCompile("^[*]+ ")
	ret := make([]string, 0)

	prevEmpty := false
	prevListed := false

	for no, line := range lines {
		// skip consecutive empty lines
		if prevEmpty && line == "" {
			continue
		}

		// skip empty lines between listings
		if prevListed && line == "" && expListed.MatchString(lines[no+1]) {
			continue
		}

		ret = append(ret, line)

		prevEmpty = false
		if line == "" {
			prevEmpty = true
		}

		prevListed = false
		if expListed.MatchString(line) {
			prevListed = true
		}
	}
	return ret
}

func write(dst string, head string, body []string) error {
	fp, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer fp.Close()

	_, err = fmt.Fprintln(fp, head)
	if err != nil {
		return err
	}

	for _, line := range body {
		_, err = fmt.Fprintln(fp, line)
		if err != nil {
			return err
		}
	}

	return nil
}
