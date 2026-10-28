import os
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
FILES = ROOT / "sbx-kit" / "files"
SPEC = ROOT / "sbx-kit" / "spec.yaml"

# SBX v2 static files only map these two subtrees into the sandbox. Anything
# else under files/ is ignored, which silently drops bundled tools.
ALLOWED_ROOTS = {"home", "workspace"}


class KitLayoutTests(unittest.TestCase):
    def test_kit_does_not_request_sbx_instruction_scaffolding(self):
        self.assertNotRegex(
            SPEC.read_text(),
            r"(?m)^agentInstructions\s*:",
            "Keep sandbox guidance in pi-agent/AGENTS.md, not SBX-generated workspace files",
        )

    def test_static_files_only_live_under_mapped_roots(self):
        misplaced = sorted(
            str(path.relative_to(FILES))
            for path in FILES.rglob("*")
            if path.is_file() and path.relative_to(FILES).parts[0] not in ALLOWED_ROOTS
        )
        self.assertEqual(
            misplaced,
            [],
            "SBX ignores static files outside files/home/ and files/workspace/",
        )

    def test_github_cli_is_installed_and_checked_during_image_build(self):
        dockerfile = (ROOT / "sbx-kit/Dockerfile").read_text()
        self.assertRegex(dockerfile, r"apt-get install\b[^\n]*\bgh\s*&&")
        self.assertIn("gh --version &&", dockerfile)

    def test_search_model_download_uses_pinned_revision_and_system_tls(self):
        dockerfile = (ROOT / "sbx-kit/Dockerfile").read_text()
        self.assertRegex(dockerfile, r"ARG COLGREP_MODEL_REVISION=[0-9a-f]{40}\n")
        self.assertIn("/opt/pissbox/models/LateOn-Code-edge", dockerfile)
        self.assertIn("/resolve/${COLGREP_MODEL_REVISION}/${file}", dockerfile)
        self.assertIn("--proto-redir '=https'", dockerfile)
        self.assertLess(dockerfile.index("update-ca-certificates;"),
                        dockerfile.index("RUN model_dir="))
        for name in ("tokenizer.json", "config_sentence_transformers.json", "config.json",
                     "onnx_config.json", "model_int8.onnx", "model.onnx"):
            self.assertIn(name, dockerfile)

    def test_optional_ca_is_installed_as_root_before_npm(self):
        dockerfile = (ROOT / "sbx-kit/Dockerfile").read_text()
        ca_step = dockerfile.index("RUN --mount=type=secret,id=custom_ca")
        self.assertEqual(dockerfile[:ca_step].rsplit("USER ", 1)[1].splitlines()[0], "root")
        self.assertIn("if [ -f /run/secrets/custom_ca ]; then", dockerfile)
        self.assertIn("install -m 0644 /run/secrets/custom_ca", dockerfile)
        self.assertLess(ca_step, dockerfile.index("update-ca-certificates;"))
        self.assertLess(dockerfile.index("update-ca-certificates;"),
                        dockerfile.index('npm install -g "@earendil-works/pi-coding-agent@${version}"'))
        self.assertNotIn("COPY ca.crt", dockerfile)

    def test_node_is_mise_managed_and_pi_has_a_version_independent_npm_prefix(self):
        dockerfile = (ROOT / "sbx-kit/Dockerfile").read_text()
        node_step = dockerfile.index("RUN mise use -g node@26.9.0 pnpm@12.15.0")
        self.assertLess(dockerfile.index("update-ca-certificates;"), node_step)
        self.assertLess(node_step,
                        dockerfile.index('npm install -g "@earendil-works/pi-coding-agent@${version}"'))
        self.assertEqual(dockerfile[:node_step].rsplit("USER ", 1)[1].splitlines()[0], "agent")
        self.assertIn('test "$(node --version)" = "v26.9.0"', dockerfile)
        self.assertIn("ENV NPM_CONFIG_PREFIX=/opt/pissbox/npm", dockerfile)
        self.assertLess(dockerfile.index('chown -R agent:agent "$MISE_DATA_DIR" "$NPM_CONFIG_PREFIX"'),
                        node_step)
        self.assertNotIn("/opt/pissbox/node", dockerfile)
        self.assertNotIn("/var/tmp/node.tar.xz", dockerfile)

    def test_mise_is_verified_and_pnpm_uses_project_aware_shims(self):
        dockerfile = (ROOT / "sbx-kit/Dockerfile").read_text()
        mise_step = dockerfile.index("RUN version=2026.10.21")
        extract = dockerfile.index("tar -xJf /var/tmp/mise.tar.xz")
        pnpm_step = dockerfile.index("RUN mise use -g node@26.9.0 pnpm@12.15.0")
        self.assertIn("mise-v${version}-linux-${arch}.tar.xz", dockerfile)
        self.assertIn("amd64) arch=x64; checksum=e1743bb7628927f93d4ffb2c4e2c4f71186307612f59ebc7f85d52e3a7f42461", dockerfile)
        self.assertIn("arm64) arch=arm64; checksum=3a7ecb96f2d9fbf5cb05631f1da02699d49e592b3ed3adfd5a7fce7420d43ee9", dockerfile)
        self.assertLess(dockerfile.index("update-ca-certificates;"), mise_step)
        self.assertLess(dockerfile.index("sha256sum -c -", mise_step), extract)
        self.assertIn("--strip-components=2 mise/bin/mise", dockerfile)
        self.assertIn("ENV MISE_DATA_DIR=/opt/pissbox/mise", dockerfile)
        self.assertIn('ENV PATH="/opt/pissbox/mise/shims:/opt/pissbox/npm/bin:${PATH}"', dockerfile)
        self.assertLess(dockerfile.index('chown -R agent:agent "$MISE_DATA_DIR" "$NPM_CONFIG_PREFIX"'), pnpm_step)
        self.assertEqual(dockerfile[:pnpm_step].rsplit("USER ", 1)[1].splitlines()[0], "agent")
        self.assertIn('test "$(pnpm --version)" = "12.15.0"', dockerfile)
        self.assertNotIn("npm install -g pnpm", dockerfile)
        self.assertIn("for tool in node npm npx pnpm pnpx; do", dockerfile)
        self.assertIn('ln -sf "/opt/pissbox/mise/shims/$tool" "/usr/local/bin/$tool"', dockerfile)

    def test_project_toml_parser_is_pinned_verified_and_installed_after_ca(self):
        dockerfile = (ROOT / "sbx-kit/Dockerfile").read_text()
        self.assertIn("RUN version=2.4.3", dockerfile)
        self.assertIn("tomljson_${version}_linux_${TARGETARCH}.tar.xz", dockerfile)
        self.assertIn("amd64) checksum=70282ec360a01cd6895a01267110338c6dae00d73d0c00a83b5bdc3b1884d083", dockerfile)
        self.assertIn("arm64) checksum=1e5bb8e8f42dded2a811594f0b15e01ab0546d1c2c3b8b388252d3b6e2084ab0", dockerfile)
        self.assertIn("xz-utils jq", dockerfile)
        verify = dockerfile.index("sha256sum -c -", dockerfile.index("RUN version=2.4.3"))
        extract = dockerfile.index("tar -xJf /var/tmp/tomljson.tar.xz -C /usr/local/bin tomljson")
        self.assertLess(dockerfile.index("update-ca-certificates;"), verify)
        self.assertLess(verify, extract)

    def test_bundled_model_default_is_seeded_after_shared_links(self):
        dockerfile = (ROOT / "sbx-kit/Dockerfile").read_text()
        spec = SPEC.read_text()
        self.assertIn('{"default_model":"/opt/pissbox/models/LateOn-Code-edge"}', dockerfile)
        self.assertIn("> /opt/pissbox/colgrep-config.json", dockerfile)
        self.assertLess(spec.index("link-shared-directories.sh"),
                        spec.index("cp /opt/pissbox/colgrep-config.json"))

    def test_model_seed_preserves_existing_config_and_indexes(self):
        command = next(line.split("command: '", 1)[1][:-1]
                       for line in SPEC.read_text().splitlines()
                       if "cp /opt/pissbox/colgrep-config.json" in line)
        fixtures = ROOT / ".cache/test-fixtures"
        fixtures.mkdir(parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(dir=fixtures) as directory:
            home = Path(directory)
            config_dir = home / ".local/share/colgrep"
            config_dir.mkdir(parents=True)
            source = home / "image-default.json"
            default = '{"default_model":"/opt/pissbox/models/LateOn-Code-edge"}\n'
            source.write_text(default)
            command = command.replace("/opt/pissbox/colgrep-config.json", '"' + str(source) + '"')
            config = config_dir / "config.json"
            index = config_dir / "indices/retained"
            index.parent.mkdir()
            index.write_text("existing index")

            def seed():
                subprocess.run(["sh", "-c", command], check=True,
                               env={**os.environ, "HOME": str(home)})

            seed()
            self.assertEqual(config.read_text(), default)
            config.write_text('{"default_model":"custom/model","default_k":7}')
            seed()
            self.assertEqual(config.read_text(), '{"default_model":"custom/model","default_k":7}')
            self.assertEqual(index.read_text(), "existing index")
            config.unlink()
            config.symlink_to(home / "missing-config")
            seed()
            self.assertTrue(config.is_symlink())
            self.assertFalse((home / "missing-config").exists())

    def test_npm_uses_shared_cache_after_links_are_created(self):
        spec = SPEC.read_text()
        cache_command = 'npm config set cache "$HOME/.cache/npm" --location=user'
        self.assertIn(cache_command, spec)
        self.assertLess(spec.index("link-shared-directories.sh"), spec.index(cache_command))


if __name__ == "__main__":
    unittest.main()
