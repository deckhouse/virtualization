/*
Copyright 2026 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cloudinit

import (
	"bytes"
	"compress/gzip"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCloudInit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CloudInit Suite")
}

var _ = Describe("ValidateUserData", func() {
	DescribeTable("says nothing about payloads cloud-init understands",
		func(userData string) {
			Expect(ValidateUserData([]byte(userData))).To(BeEmpty())
		},
		Entry("a cloud-config", "#cloud-config\nusers:\n  - name: cloud\n"),
		Entry("a cloud-config with a document separator", "#cloud-config\n---\npackages:\n  - nginx\n"),
		Entry("a cloud-config with nothing but the header", "#cloud-config"),
		Entry("a cloud-config after leading blank lines", "\n\n#cloud-config\nhostname: vm\n"),
		Entry("a cloud-config with CRLF line endings", "#cloud-config\r\nhostname: vm\r\n"),
		Entry("a cloud-config after a byte order mark", "\ufeff#cloud-config\nhostname: vm\n"),
		Entry("a shell script", "#!/bin/bash\necho hello\n"),
		Entry("an include file", "#include\nhttps://example.com/config\n"),
		Entry("an include-once file", "#include-once\nhttps://example.com/config\n"),
		Entry("a boot hook", "#cloud-boothook\n#!/bin/sh\necho early\n"),
		Entry("a part handler", "#part-handler\ndef list_types():\n    pass\n"),
		Entry("an upstart job", "#upstart-job\nstart on stopped rc\nexec /bin/true\n"),
		Entry("a cloud-config archive", "#cloud-config-archive\n- type: text/cloud-boothook\n  content: |\n    #!/bin/sh\n"),
		Entry("a cloud-config archive of bare strings", "#cloud-config-archive\n- \"#cloud-config\\nhostname: vm\\n\"\n"),
		Entry("a cloud-config jsonp patch", "#cloud-config-jsonp\n[{\"op\": \"add\", \"path\": \"/hostname\", \"value\": \"vm\"}]\n"),
		Entry("a MIME archive", "MIME-Version: 1.0\nContent-Type: multipart/mixed; boundary=\"==B==\"\n"),
		Entry("a MIME archive declaring its type first", "Content-Type: multipart/mixed; boundary=\"==B==\"\nMIME-Version: 1.0\n"),
		Entry("a MIME archive with only a type header", "Content-Type: multipart/mixed; boundary=\"==B==\"\n"),
	)

	DescribeTable("matches the header the way cloud-init does, ignoring case",
		func(userData string) {
			Expect(ValidateUserData([]byte(userData))).To(BeEmpty())
		},
		Entry("an upper case cloud-config", "#Cloud-Config\nhostname: vm\n"),
		Entry("an upper case jinja template", "## Template: Jinja\n#cloud-config\nhostname: {{ v1.local_hostname }}\n"),
		Entry("a lower case MIME version", "mime-version: 1.0\n"),
	)

	It("says nothing about a Jinja template, whose placeholders are not YAML yet", func() {
		userData := "## template: jinja\n#cloud-config\nhostname: {{ v1.local_hostname }}\n"

		Expect(ValidateUserData([]byte(userData))).To(BeEmpty())
	})

	It("says nothing about a gzipped payload, which it cannot read", func() {
		var buf bytes.Buffer
		w := gzip.NewWriter(&buf)
		_, err := w.Write([]byte("#cloud-config\nusers: []\n"))
		Expect(err).NotTo(HaveOccurred())
		Expect(w.Close()).To(Succeed())

		Expect(ValidateUserData(buf.Bytes())).To(BeEmpty())
	})

	DescribeTable("reports an empty payload",
		func(userData string) {
			Expect(ValidateUserData([]byte(userData))).To(ConsistOf(ContainSubstring("empty")))
		},
		Entry("no bytes at all", ""),
		Entry("only whitespace", "  \n\t\n"),
	)

	DescribeTable("reports a cloud-config cloud-init cannot parse",
		func(userData string) {
			Expect(ValidateUserData([]byte(userData))).To(ConsistOf(ContainSubstring("not a valid cloud-config")))
		},
		Entry("broken indentation", "#cloud-config\nusers:\n - name: cloud\n   groups: sudo\n  shell: /bin/bash\n"),
		Entry("an unclosed quote", "#cloud-config\nhostname: \"vm\n"),
		Entry("a tab used for indentation", "#cloud-config\nusers:\n\t- name: cloud\n"),
		Entry("a list instead of a mapping", "#cloud-config\n- one\n- two\n"),
		Entry("a bare scalar", "#cloud-config\njust a string\n"),
	)

	DescribeTable("reports an archive that is not a list of entries",
		func(userData, header string) {
			Expect(ValidateUserData([]byte(userData))).To(ConsistOf(ContainSubstring("declares " + header)))
		},
		Entry("a cloud-config archive holding a mapping", "#cloud-config-archive\nhostname: vm\n", "#cloud-config-archive"),
		Entry("a cloud-config archive that is not parseable", "#cloud-config-archive\n- type: \"text\n", "#cloud-config-archive"),
		Entry("a jsonp patch holding a mapping", "#cloud-config-jsonp\n{\"op\": \"add\"}\n", "#cloud-config-jsonp"),
	)

	It("does not mistake an archive for a plain cloud-config", func() {
		// #cloud-config-archive starts with #cloud-config, and a list is exactly
		// what a plain cloud-config may not be.
		Expect(ValidateUserData([]byte("#cloud-config-archive\n- one\n- two\n"))).To(BeEmpty())
	})

	It("keeps the message short for a large malformed payload", func() {
		userData := "#cloud-config\nhostname: \"" + string(bytes.Repeat([]byte("x"), 4096))

		warnings := ValidateUserData([]byte(userData))

		Expect(warnings).To(HaveLen(1))
		Expect(warnings[0]).NotTo(ContainSubstring("xxxxxxxxxxxxxxxxxxxx"))
	})

	DescribeTable("reports a jinja header spelled the way cloud-init does not match",
		// cloud-init compares the start of the payload against the headers
		// literally, so only "## template: jinja" makes it a template.
		func(userData string) {
			warnings := ValidateUserData([]byte(userData))

			Expect(warnings).To(HaveLen(1))
			Expect(warnings[0]).To(ContainSubstring("ignore the payload"))
			Expect(warnings[0]).To(ContainSubstring(`"## template: jinja"`))
		},
		Entry("no space after the hashes", "##template: jinja\n#cloud-config\nhostname: vm\n"),
		Entry("no space after the colon", "## template:jinja\n#cloud-config\nhostname: vm\n"),
		Entry("no spaces at all", "##template:jinja\n#cloud-config\nhostname: vm\n"),
		Entry("two spaces after the hashes", "##  template: jinja\n#cloud-config\nhostname: vm\n"),
		Entry("a tab after the colon", "## template:\tjinja\n#cloud-config\nhostname: vm\n"),
		Entry("a template type cloud-init does not read", "## template: basic\n#cloud-config\nhostname: vm\n"),
		Entry("mixed case and no space", "##Template:Jinja\n#cloud-config\nhostname: vm\n"),
	)

	It("reports a payload with no cloud-init header", func() {
		warnings := ValidateUserData([]byte("users:\n  - name: cloud\n"))

		Expect(warnings).To(HaveLen(1))
		Expect(warnings[0]).To(ContainSubstring("cloud-init will ignore it"))
		Expect(warnings[0]).To(ContainSubstring("#cloud-config"))
	})

	It("truncates the payload quoted in the message", func() {
		warnings := ValidateUserData(bytes.Repeat([]byte("a"), 4096))

		Expect(warnings).To(HaveLen(1))
		Expect(len(warnings[0])).To(BeNumerically("<", 400))
	})
})
