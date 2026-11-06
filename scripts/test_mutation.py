import argparse
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import time

ROOT = Path(__file__).resolve().parents[1]
CASES = [
    ("validation_without_field", "internal/domain/trip/errors.go", 'e.Field + ": " + e.Reason', '": " + e.Reason', "./internal/domain/trip"),
    ("validation_without_reason", "internal/domain/trip/errors.go", 'e.Field + ": " + e.Reason', 'e.Field + ": "', "./internal/domain/trip"),
]
for action, event in [("create", "created"), ("finish", "completed")]:
    file = f"internal/usecase/trip/{action}/usecase.go"
    package = f"./internal/usecase/trip/{action}"
    CASES.extend([
        (f"{action}_without_utc", file, ".Now().UTC()", ".Now()", package),
        (f"{action}_keep_nanoseconds", file, "Truncate(time.Microsecond)", "Truncate(time.Nanosecond)", package),
        (f"{action}_without_log", file, f'\ts.logger.InfoContext(ctx, "trip {event}", "trip_id", '+("created.ID" if action == "create" else "id")+")\n", "", package),
    ])


def run_test(directory, package):
    started = time.monotonic()
    result = subprocess.run(
        ["go", "test", "-race", "-count=1", "-json", package],
        cwd=directory, capture_output=True, text=True, timeout=60,
    )
    events = []
    for line in result.stdout.splitlines():
        try:
            events.append(json.loads(line))
        except json.JSONDecodeError:
            pass
    failures = [event.get("Output", "") for event in events if event.get("Output")]
    failed_tests = sorted({event["Test"] for event in events if event.get("Action") == "fail" and event.get("Test")})
    if result.returncode != 0 and not failed_tests:
        raise RuntimeError(f"Build or infrastructure failure for {package}:\n{result.stdout}\n{result.stderr}")
    return result.returncode, failed_tests, "".join(failures), round(time.monotonic() - started, 3)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--phase", choices=["before", "after"], required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--summary-prefix", type=Path)
    args = parser.parse_args()
    if args.summary_prefix is not None:
        prefix = args.summary_prefix if args.summary_prefix.is_absolute() else ROOT / args.summary_prefix
        parts = [json.loads(Path(str(prefix) + "-" + scope + ".json").read_text()) for scope in ["domain", "usecase", "supplemental"]]
        summary = {key: sum(part[key] for part in parts) for key in ["mutants_total", "mutants_killed", "mutants_lived"]}
        output = args.output if args.output.is_absolute() else ROOT / args.output
        output.write_text(json.dumps(summary, indent=2) + "\n")
        print(json.dumps(summary))
        if args.phase == "after" and summary["mutants_killed"] != summary["mutants_total"]:
            raise SystemExit(1)
        return
    report = {"phase": args.phase, "command": "go test -race -count=1 -json <package>", "mutations": []}
    ignored = shutil.ignore_patterns(".git", ".env", ".tripgo", ".idea", ".playwright-cli", "bin", "output", "docs", "coverage.out", "__pycache__")
    with tempfile.TemporaryDirectory(prefix="tripgo-mutation-") as temporary:
        working = Path(temporary) / "module"
        shutil.copytree(ROOT, working, ignore=ignored)
        if args.phase == "before":
            snapshots = ROOT / "docs/mutation/baseline-tests"
            for snapshot in snapshots.rglob("*.go.txt"):
                relative = snapshot.relative_to(snapshots).with_suffix("")
                shutil.copyfile(snapshot, working / relative)
        for package in sorted({case[4] for case in CASES}):
            code, _, output, _ = run_test(working, package)
            if code:
                raise RuntimeError(f"Unmodified tests fail: {package}\n{output}")
        for name, file, original, replacement, package in CASES:
            path = working / file
            source = path.read_text()
            if source.count(original) != 1:
                raise RuntimeError(f"Mutation {name} must match exactly once")
            path.write_text(source.replace(original, replacement, 1))
            try:
                code, failed_tests, output, elapsed = run_test(working, package)
            finally:
                path.write_text(source)
            status = "KILLED" if code else "LIVED"
            report["mutations"].append({
                "id": name, "file": file, "original": original, "replacement": replacement,
                "status": status, "failed_tests": failed_tests, "elapsed_seconds": elapsed,
                "test_output": output if code else "",
            })
            print(f"{status:6} {name}", flush=True)
    report["mutants_total"] = len(report["mutations"])
    report["mutants_killed"] = sum(m["status"] == "KILLED" for m in report["mutations"])
    report["mutants_lived"] = sum(m["status"] == "LIVED" for m in report["mutations"])
    output = args.output if args.output.is_absolute() else ROOT / args.output
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, indent=2, ensure_ascii=False) + "\n")
    print(f"Total: {report['mutants_total']}, killed: {report['mutants_killed']}, lived: {report['mutants_lived']}")
    if args.phase == "after" and report["mutants_lived"]:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
