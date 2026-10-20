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
                        dockerfile.index("npm install -g pnpm"))
        self.assertNotIn("COPY ca.crt", dockerfile)

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
