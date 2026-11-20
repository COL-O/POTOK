#!/usr/bin/env python3
"""Исполняет заявки на статус карточки доски «Круглый стол ПОТОКА».

Почему так: токен GitHub Actions умеет ЧИТАТЬ Projects v2, но не писать —
`updateProjectV2ItemFieldValue` отвечает `Resource not accessible by integration`
(проверено 20.11.2026). Поэтому мост `scripts/sfera_inbox.py` директивой
`#### ▶ board 23 В работе` кладёт заявку файлом `board/requests/<N>-<ts>.md`,
а применяет её дом — этот скрипт, у которого есть право записи.

Запуск (на машине, где лежит репозиторий и авторизован gh):
    python3 scripts/board_apply.py            # применить все заявки
    python3 scripts/board_apply.py --dry-run  # только показать

Заявка: `{"issue": 23, "status": "В работе"}`.
После применения файл удаляется, чтобы заявка не сработала дважды.
"""
import argparse
import json
import pathlib
import subprocess
import sys

GH = "gh"
OWNER = "COL-O"
PROJECT_NUMBER = 5
PROJECT_ID = "PVT_kwHOFUYpfM4BsAC8"
STATUS_FIELD = "PVTSSF_lAHOFUYpfM4BsAC8zhqQItg"
STATUSES = {
    "новое": "8db2bba4",
    "в работе": "934fb0d8",
    "на проверке": "5b2cf21b",
    "готово": "c331d1b0",
    "отложено": "a65399be",
}

ROOT = pathlib.Path(__file__).resolve().parent.parent
REQUESTS = ROOT / "board" / "requests"


def gh(args: list[str]) -> str:
    r = subprocess.run([GH, *args], capture_output=True, text=True)
    if r.returncode != 0:
        raise RuntimeError((r.stderr or r.stdout).strip()[:300])
    return r.stdout


def item_for(issue: int) -> str | None:
    query = (
        '{user(login:"' + OWNER + '"){projectV2(number:' + str(PROJECT_NUMBER) + '){'
        'items(first:100){nodes{id content{... on Issue{number}}}}}}}'
    )
    out = gh(["api", "graphql", "-f", "query=" + query])
    data = json.loads(out)["data"]["user"]["projectV2"]["items"]["nodes"]
    for node in data:
        if (node.get("content") or {}).get("number") == issue:
            return node["id"]
    return None


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--dry-run", action="store_true")
    args = ap.parse_args()

    if not REQUESTS.exists():
        print("заявок нет")
        return 0
    files = sorted(REQUESTS.glob("*.md"))
    if not files:
        print("заявок нет")
        return 0

    ok = 0
    for f in files:
        try:
            req = json.loads(f.read_text(encoding="utf-8"))
        except json.JSONDecodeError:
            print(f"{f.name}: не JSON — пропущено")
            continue
        issue = int(req.get("issue") or 0)
        status = str(req.get("status") or "").strip().lower()
        option = STATUSES.get(status)
        if not issue or not option:
            print(f"{f.name}: непонятная заявка {req} — пропущено")
            continue
        if args.dry_run:
            print(f"{f.name}: #{issue} → {status} (dry-run)")
            continue
        try:
            item = item_for(issue)
            if not item:
                print(f"{f.name}: карточки задачи #{issue} на доске нет — заявка оставлена")
                continue
            gh(["api", "graphql", "-f",
                "query=mutation($p:ID!,$i:ID!,$f:ID!,$o:String!){updateProjectV2ItemFieldValue("
                "input:{projectId:$p,itemId:$i,fieldId:$f,value:{singleSelectOptionId:$o}})"
                "{projectV2Item{id}}}",
                "-f", f"p={PROJECT_ID}", "-f", f"i={item}", "-f", f"f={STATUS_FIELD}",
                "-f", f"o={option}"])
            f.unlink()
            ok += 1
            print(f"#{issue} → {status}")
        except RuntimeError as e:
            print(f"{f.name}: не применено — {e}")

    if ok and not args.dry_run:
        try:
            subprocess.run(["git", "-C", str(ROOT), "add", "-A", "board/requests"], check=True)
            subprocess.run(["git", "-C", str(ROOT), "-c", "user.name=Русло",
                            "-c", "user.email=ruslo@potok.local",
                            "commit", "-qm", "board: заявки на статус исполнены"], check=True)
        except subprocess.CalledProcessError as e:
            print(f"заявки исполнены, но не закоммичены: {e}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
