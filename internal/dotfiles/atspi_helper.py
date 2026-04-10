#!/usr/bin/env python3
import argparse
import json
import sys
import time

try:
    import pyatspi
except ImportError:
    print(json.dumps({"error": "pyatspi not found. Please install python-pyatspi or equivalent."}))
    sys.exit(1)


def normalize(value):
    return (value or "").strip().lower()


def match_value(value, needle, exact=False):
    haystack = normalize(value)
    target = normalize(needle)
    if not target:
        return True
    if exact:
        return haystack == target
    return target in haystack


def safe_role(obj):
    try:
        return obj.getRoleName()
    except Exception:
        return ""


def safe_actions(obj):
    actions = []
    try:
        action = obj.queryAction()
        for idx in range(action.nActions):
            actions.append(action.getName(idx))
    except Exception:
        pass
    return actions


def element_to_dict(obj, path="", depth=0, max_depth=5):
    if depth > max_depth:
        return {
            "path": path,
            "name": obj.name,
            "role": safe_role(obj),
            "depth_limit": True,
        }

    res = {
        "path": path,
        "name": obj.name,
        "role": safe_role(obj),
        "description": getattr(obj, "description", None),
    }

    try:
        states = obj.getState().get_states()
        res["states"] = [pyatspi.state_to_name(s) for s in states]
    except Exception:
        pass

    actions = safe_actions(obj)
    if actions:
        res["actions"] = actions

    if depth < max_depth:
        children = []
        for idx in range(getattr(obj, "childCount", 0)):
            child = obj.getChildAtIndex(idx)
            if child:
                child_path = str(idx) if path == "" else f"{path}/{idx}"
                children.append(element_to_dict(child, child_path, depth + 1, max_depth))
        res["children"] = children

    return res


def iter_apps():
    reg = pyatspi.Registry
    for idx in range(reg.getAppCount()):
        yield idx, reg.getApp(idx)


def list_apps():
    apps = []
    for idx, app in iter_apps():
        apps.append(
            {
                "id": idx,
                "name": app.name,
                "role": safe_role(app),
                "child_count": getattr(app, "childCount", 0),
            }
        )
    return {"apps": apps}


def find_app(app_name):
    for idx, app in iter_apps():
        if match_value(app.name, app_name, exact=True):
            return idx, app
    for idx, app in iter_apps():
        if match_value(app.name, app_name, exact=False):
            return idx, app
    return None, None


def locate_by_path(root, path):
    current = root
    if not path:
        return current
    for raw in path.split("/"):
        if raw == "":
            continue
        idx = int(raw)
        current = current.getChildAtIndex(idx)
        if current is None:
            return None
    return current


def search_element(root, search_name, role=None, exact=False, path=None):
    if path:
        found = locate_by_path(root, path)
        if found is None:
            return None
        if search_name and not match_value(found.name, search_name, exact):
            return None
        if role and not match_value(safe_role(found), role, exact=True):
            return None
        return found

    def visit(obj):
        if match_value(obj.name, search_name, exact) and (
            role is None or match_value(safe_role(obj), role, exact=True)
        ):
            return obj
        for idx in range(getattr(obj, "childCount", 0)):
            child = obj.getChildAtIndex(idx)
            if child is None:
                continue
            res = visit(child)
            if res is not None:
                return res
        return None

    return visit(root)


def get_tree(app_name, max_depth=5):
    idx, app = find_app(app_name)
    if app is None:
        return {"matched": False, "error": f"App {app_name} not found"}
    return {
        "matched": True,
        "app": {"id": idx, "name": app.name, "role": safe_role(app)},
        "tree": element_to_dict(app, "", 0, max_depth),
    }


def find_element(app_name, search_name, role=None, exact=False, path=None):
    idx, app = find_app(app_name)
    if app is None:
        return {"matched": False, "error": f"App {app_name} not found"}

    found = search_element(app, search_name, role, exact, path)
    if found is None:
        return {"matched": False, "error": f"Element not found in {app_name}"}

    return {
        "matched": True,
        "app": {"id": idx, "name": app.name, "role": safe_role(app)},
        "element": element_to_dict(found, "", 0, 0),
    }


def click_element(app_name, search_name, role=None, exact=False, path=None):
    idx, app = find_app(app_name)
    if app is None:
        return {"matched": False, "clicked": False, "error": f"App {app_name} not found"}

    found = search_element(app, search_name, role, exact, path)
    if found is None:
        return {"matched": False, "clicked": False, "error": f"Element not found in {app_name}"}

    try:
        action = found.queryAction()
    except Exception as exc:
        return {
            "matched": True,
            "clicked": False,
            "error": f"Element has no actionable interface: {exc}",
            "element": element_to_dict(found, "", 0, 0),
        }

    if action.nActions == 0:
        return {
            "matched": True,
            "clicked": False,
            "error": "Element exposes no actions",
            "element": element_to_dict(found, "", 0, 0),
        }

    chosen = 0
    for idx in range(action.nActions):
        name = normalize(action.getName(idx))
        if "click" in name or "press" in name or "activate" in name:
            chosen = idx
            break

    action_name = action.getName(chosen)
    try:
        action.doAction(chosen)
    except Exception as exc:
        return {
            "matched": True,
            "clicked": False,
            "action": action_name,
            "error": f"Failed to invoke action: {exc}",
            "element": element_to_dict(found, "", 0, 0),
        }

    return {
        "matched": True,
        "clicked": True,
        "action": action_name,
        "app": {"id": idx, "name": app.name, "role": safe_role(app)},
        "element": element_to_dict(found, "", 0, 0),
    }


def wait_for_element(app_name, search_name, role=None, exact=False, path=None, timeout=5):
    start = time.time()
    while time.time() - start < timeout:
        res = find_element(app_name, search_name, role, exact, path)
        if res.get("matched"):
            res["waited_ms"] = int((time.time() - start) * 1000)
            return res
        time.sleep(0.35)

    return {
        "matched": False,
        "error": f"Timeout waiting for element in {app_name}",
        "waited_ms": int((time.time() - start) * 1000),
    }


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("command", choices=["list_apps", "get_tree", "find", "click", "wait"])
    parser.add_argument("--app", help="Application name")
    parser.add_argument("--name", help="Element name")
    parser.add_argument("--role", help="Element role")
    parser.add_argument("--path", help="Element child-index path such as 0/2/1")
    parser.add_argument("--exact", action="store_true", help="Require exact case-insensitive name matching")
    parser.add_argument("--depth", type=int, default=5, help="Max tree depth")
    parser.add_argument("--timeout", type=int, default=5, help="Timeout in seconds")
    args = parser.parse_args()

    if args.command == "list_apps":
        print(json.dumps(list_apps()))
        return

    if not args.app:
        print(json.dumps({"matched": False, "error": "--app is required"}))
        return

    if args.command == "get_tree":
        print(json.dumps(get_tree(args.app, args.depth)))
        return

    if args.command == "find":
        if not args.name and not args.path:
            print(json.dumps({"matched": False, "error": "--name or --path is required"}))
            return
        print(json.dumps(find_element(args.app, args.name, args.role, args.exact, args.path)))
        return

    if args.command == "click":
        if not args.name and not args.path:
            print(json.dumps({"matched": False, "clicked": False, "error": "--name or --path is required"}))
            return
        print(json.dumps(click_element(args.app, args.name, args.role, args.exact, args.path)))
        return

    if args.command == "wait":
        if not args.name and not args.path:
            print(json.dumps({"matched": False, "error": "--name or --path is required"}))
            return
        print(json.dumps(wait_for_element(args.app, args.name, args.role, args.exact, args.path, args.timeout)))
        return


if __name__ == "__main__":
    main()
