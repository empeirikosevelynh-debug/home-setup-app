import importlib.util
from pathlib import Path
import tempfile
import unittest
import subprocess
import sys
import os

spec = importlib.util.spec_from_file_location("release",Path(__file__).with_name("release.py"))
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)


class ReleaseTests(unittest.TestCase):
    def test_notices_follow_actual_linked_modules(self):
        module={"Path":"dependency","Version":"v1.0.0","Dir":"/cache/dependency"}
        packages=[{"Standard":True,"ImportPath":"fmt"},{"Module":module},{"Module":module}]
        self.assertEqual(release.linked_modules(packages),[module])

    def test_existing_output_and_dirty_source_are_refused_before_build(self):
        with tempfile.TemporaryDirectory() as tmp:
            existing = subprocess.run([sys.executable,str(Path(release.__file__)),"--output",tmp,"--go","missing-go"],capture_output=True)
            self.assertNotEqual(existing.returncode,0)
            self.assertIn(b"output already exists",existing.stderr)
            scripts=Path(tmp)/"scripts";scripts.mkdir()
            script=scripts/"release.py";script.write_text(Path(release.__file__).read_text())
            executable=Path(tmp)/"git";executable.write_text("#!/bin/sh\nprintf ' M README.md\\n'\n");executable.chmod(0o755)
            out=Path(tmp)/"new-output"
            dirty=subprocess.run([sys.executable,str(script),"--output",str(out),"--go","missing-go"],env=dict(os.environ,PATH=tmp),capture_output=True)
            self.assertNotEqual(dirty.returncode,0)
            self.assertIn(b"commit reviewed changes",dirty.stderr)
            self.assertFalse(out.exists())

    def test_version_cannot_inject_paths_or_build_flags(self):
        for value in ("../file","1.0.0 -X bad=value","1.0.0\n","","v1.0.0"):
            with self.assertRaises(ValueError): release.validate_version(value)
        self.assertEqual(release.validate_version("0.1.0-rc.1"),"0.1.0-rc.1")

    def test_module_stream_and_complete_license_text(self):
        self.assertEqual(list(release.json_stream(' {"Path":"first"}\n{"Path":"second"}')), [{"Path":"first"},{"Path":"second"}])
        with tempfile.TemporaryDirectory() as tmp:
            path=Path(tmp)/"LICENSE.md"; path.write_text("terms")
            self.assertEqual(release.license_files(tmp),[path])


if __name__ == "__main__": unittest.main()
