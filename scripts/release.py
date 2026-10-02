#!/usr/bin/env python3
"""Create local unsigned Darwin/arm64 binary and source archives from a clean commit."""
import argparse
import datetime
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parent.parent


def validate_version(version):
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?", version):
        raise ValueError("use a semantic version such as 0.1.0-rc.1")
    return version


def json_stream(text):
    decoder = json.JSONDecoder()
    while text.strip():
        value, end = decoder.raw_decode(text.lstrip())
        yield value
        text = text.lstrip()[end:]


def license_files(directory):
    directory = Path(directory)
    return sorted(p for p in directory.iterdir() if p.is_file() and
                  (p.name.upper().startswith(("LICENSE", "LICENCE", "COPYING", "NOTICE"))))


def linked_modules(packages):
    modules = {}
    for package in packages:
        module = package.get("Module")
        if module:
            if not module.get("Dir"):
                raise ValueError(f"linked module has no source directory: {module['Path']}")
            modules[module["Path"]] = module
    return [modules[path] for path in sorted(modules)]


def archive(destination, root, files, timestamp):
    with destination.open("xb") as output:
        with gzip.GzipFile(filename="", mode="wb", fileobj=output, mtime=timestamp) as compressed:
            with tarfile.open(fileobj=compressed, mode="w") as tar:
                for path in sorted(files):
                    info = tar.gettarinfo(str(path), str(path.relative_to(root)))
                    info.uid = info.gid = 0
                    info.uname = info.gname = ""
                    info.mtime = timestamp
                    with path.open("rb") as source:
                        tar.addfile(info, source)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--go", default="go", help="Go compiler executable")
    parser.add_argument("--version", default="0.1.0-rc.1")
    parser.add_argument("--output", type=Path, help="new output directory; existing directories are refused")
    args = parser.parse_args()
    version = validate_version(args.version)
    destination = (args.output or ROOT/"dist"/version).resolve()
    if destination.exists():
        raise ValueError("output already exists; choose a new directory")
    env = dict(os.environ, GOTOOLCHAIN="local", GOFLAGS="", GOWORK="off")

    def run(*command, binary=False, extra=None):
        return subprocess.check_output(command, cwd=ROOT, env=env if extra is None else env|extra,
                                       text=not binary)

    if run("git", "status", "--porcelain").strip():
        raise ValueError("commit reviewed changes before packaging; source and binary must match")
    commit = run("git", "rev-parse", "HEAD").strip()
    timestamp = int(run("git", "show", "-s", "--format=%ct", "HEAD"))
    built = datetime.datetime.fromtimestamp(timestamp, datetime.timezone.utc).isoformat()
    compiler = run(args.go, "version").strip()
    run(args.go, "mod", "download")
    modules = linked_modules(json_stream(run(args.go, "list", "-mod=readonly", "-deps", "-json", "./cmd/golden-setup")))
    destination.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".golden-release-", dir=destination.parent) as temporary:
        staging = Path(temporary)
        name = f"golden-gate-setup-{version}-darwin-arm64"
        package = staging/name
        package.mkdir()
        metadata = {"version": version, "commit": commit, "built": built, "compiler": compiler,
                    "target": "darwin/arm64", "signed": False, "notarized": False,
                    "modules": [{k: m[k] for k in ("Path", "Version", "Sum", "GoModSum") if k in m} for m in modules]}
        flags = " ".join(f"-X golden-gate-setup/internal/buildinfo.{key}={value}" for key,value in
                         (("Version",version),("Commit",commit),("Built",built)))
        run(args.go,"build","-mod=readonly","-trimpath","-ldflags",flags,"-o",str(package/"golden-setup"),
            "./cmd/golden-setup",extra={"GOOS":"darwin","GOARCH":"arm64","CGO_ENABLED":"0"})
        for path in ("LICENSE","README.md","THIRD_PARTY.md","CHANGELOG.md","SECURITY.md","CONTRIBUTING.md"):
            shutil.copyfile(ROOT/path,package/path)
        fisher = package/"internal/apply/assets/FISHER-LICENSE.md"
        fisher.parent.mkdir(parents=True)
        shutil.copyfile(ROOT/"internal/apply/assets/FISHER-LICENSE.md",fisher)
        shutil.copytree(ROOT/"docs",package/"docs",ignore=shutil.ignore_patterns("superpowers","source-fingerprints.json","design-review.json","source-review.md"))
        notices = ["Third-party licenses included with this binary.\n"]
        for module in modules:
            if module.get("Main"): continue
            paths = license_files(module["Dir"])
            if not paths:
                raise ValueError(f"missing dependency license: {module['Path']}")
            notices.append(f"\n=== {module['Path']} {module.get('Version','')} ===\n")
            for path in paths:
                notices.append(f"\n--- {path.name} ---\n"+path.read_text(errors="strict"))
        goroot = Path(run(args.go,"env","GOROOT").strip())
        for path in (goroot/"LICENSE",goroot/"PATENTS",ROOT/"internal/apply/assets/FISHER-LICENSE.md"):
            if path.exists(): notices.append(f"\n=== {path.name} ===\n"+path.read_text())
        (package/"THIRD_PARTY_LICENSES.txt").write_text("\n".join(notices))
        (package/"BUILDINFO.json").write_text(json.dumps(metadata,indent=2)+"\n")
        artifacts = staging/"artifacts"
        artifacts.mkdir()
        archive(artifacts/(name+".tar.gz"),staging,[p for p in package.rglob("*") if p.is_file()],timestamp)
        source = run("git","archive","--format=tar",f"--prefix=golden-gate-setup-{version}-source/",commit,binary=True)
        with (artifacts/f"golden-gate-setup-{version}-source.tar.gz").open("xb") as output:
            with gzip.GzipFile(filename="",mode="wb",fileobj=output,mtime=timestamp) as compressed:
                compressed.write(source)
        (artifacts/"BUILDINFO.json").write_text(json.dumps(metadata,indent=2)+"\n")
        sums = [f"{hashlib.sha256(p.read_bytes()).hexdigest()}  {p.name}" for p in sorted(artifacts.iterdir())]
        (artifacts/"SHA256SUMS").write_text("\n".join(sums)+"\n")
        artifacts.rename(destination)
    print(destination)


if __name__ == "__main__":
    main()
