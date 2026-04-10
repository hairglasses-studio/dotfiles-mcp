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


def path_to_ref(path):
    if not path:
        return "ref_root"
    return "ref_" + path.replace("/", "_")


def ref_to_path(ref_id):
    value = normalize(ref_id)
    if not value:
        return None
    if value == "ref_root":
        return ""
    if value.startswith("ref_"):
        return ref_id[4:].replace("_", "/")
    return None


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


def safe_states(obj):
    try:
        return [pyatspi.state_to_name(s) for s in obj.getState().get_states()]
    except Exception:
        return []


def state_matches(obj, required_states):
    if not required_states:
        return True
    current = {normalize(state) for state in safe_states(obj)}
    for state in required_states:
        if normalize(state) not in current:
            return False
    return True


def parse_states(values):
    states = []
    for value in values or []:
        for part in str(value).split(","):
            part = part.strip()
            if part:
                states.append(part)
    return states


def element_to_dict(obj, path="", depth=0, max_depth=5):
    if depth > max_depth:
        return {
            "path": path,
            "ref": path_to_ref(path),
            "name": obj.name,
            "role": safe_role(obj),
            "depth_limit": True,
        }

    res = {
        "path": path,
        "ref": path_to_ref(path),
        "name": obj.name,
        "role": safe_role(obj),
        "description": getattr(obj, "description", None),
    }

    states = safe_states(obj)
    if states:
        res["states"] = states

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


def search_element(root, search_name, role=None, exact=False, path=None, ref_id=None, states=None):
    if ref_id:
        path = ref_to_path(ref_id)
    if path is not None and path != "":
        found = locate_by_path(root, path)
        if found is None:
            return None, None
        if search_name and not match_value(found.name, search_name, exact):
            return None, None
        if role and not match_value(safe_role(found), role, exact=True):
            return None, None
        if not state_matches(found, states):
            return None, None
        return found, path
    if path == "":
        if search_name and not match_value(root.name, search_name, exact):
            return None, None
        if role and not match_value(safe_role(root), role, exact=True):
            return None, None
        if not state_matches(root, states):
            return None, None
        return root, ""

    def visit(obj, current_path=""):
        if match_value(obj.name, search_name, exact) and (
            role is None or match_value(safe_role(obj), role, exact=True)
        ) and state_matches(obj, states):
            return obj, current_path
        for idx in range(getattr(obj, "childCount", 0)):
            child = obj.getChildAtIndex(idx)
            if child is None:
                continue
            child_path = str(idx) if current_path == "" else f"{current_path}/{idx}"
            res, res_path = visit(child, child_path)
            if res is not None:
                return res, res_path
        return None, None

    return visit(root, "")


def choose_action_index(action_iface, requested_action=None):
    if action_iface.nActions == 0:
        return None, None

    if requested_action:
        target = normalize(requested_action)
        for idx in range(action_iface.nActions):
            if normalize(action_iface.getName(idx)) == target:
                return idx, action_iface.getName(idx)
        for idx in range(action_iface.nActions):
            if target in normalize(action_iface.getName(idx)):
                return idx, action_iface.getName(idx)
        return None, None

    for idx in range(action_iface.nActions):
        name = normalize(action_iface.getName(idx))
        if "click" in name or "press" in name or "activate" in name:
            return idx, action_iface.getName(idx)
    return 0, action_iface.getName(0)


def get_tree(app_name, max_depth=5):
    idx, app = find_app(app_name)
    if app is None:
        return {"matched": False, "error": f"App {app_name} not found"}
    return {
        "matched": True,
        "app": {"id": idx, "name": app.name, "role": safe_role(app)},
        "tree": element_to_dict(app, "", 0, max_depth),
    }


def find_element(app_name, search_name, role=None, exact=False, path=None, ref_id=None, states=None):
    idx, app = find_app(app_name)
    if app is None:
        return {"matched": False, "error": f"App {app_name} not found"}

    found, found_path = search_element(app, search_name, role, exact, path, ref_id, states)
    if found is None:
        return {"matched": False, "error": f"Element not found in {app_name}"}

    return {
        "matched": True,
        "app": {"id": idx, "name": app.name, "role": safe_role(app)},
        "element": element_to_dict(found, found_path or "", 0, 0),
    }


def invoke_element_action(
    app_name,
    search_name,
    role=None,
    exact=False,
    path=None,
    ref_id=None,
    states=None,
    requested_action=None,
):
    idx, app = find_app(app_name)
    if app is None:
        return {
            "matched": False,
            "invoked": False,
            "clicked": False,
            "error": f"App {app_name} not found",
        }

    found, found_path = search_element(app, search_name, role, exact, path, ref_id, states)
    if found is None:
        return {
            "matched": False,
            "invoked": False,
            "clicked": False,
            "error": f"Element not found in {app_name}",
        }

    try:
        action = found.queryAction()
    except Exception as exc:
        return {
            "matched": True,
            "invoked": False,
            "clicked": False,
            "error": f"Element has no actionable interface: {exc}",
            "element": element_to_dict(found, found_path or "", 0, 0),
        }

    chosen, action_name = choose_action_index(action, requested_action)
    if chosen is None:
        return {
            "matched": True,
            "invoked": False,
            "clicked": False,
            "error": f"Requested action not found: {requested_action}",
            "element": element_to_dict(found, found_path or "", 0, 0),
        }

    try:
        action.doAction(chosen)
    except Exception as exc:
        return {
            "matched": True,
            "invoked": False,
            "clicked": False,
            "action": action_name,
            "error": f"Failed to invoke action: {exc}",
            "element": element_to_dict(found, found_path or "", 0, 0),
        }

    return {
        "matched": True,
        "invoked": True,
        "clicked": requested_action is None,
        "action": action_name,
        "app": {"id": idx, "name": app.name, "role": safe_role(app)},
        "element": element_to_dict(found, found_path or "", 0, 0),
    }


def click_element(app_name, search_name, role=None, exact=False, path=None, ref_id=None, states=None):
    return invoke_element_action(app_name, search_name, role, exact, path, ref_id, states)


def wait_for_element(app_name, search_name, role=None, exact=False, path=None, ref_id=None, states=None, timeout=5):
    start = time.time()
    while time.time() - start < timeout:
        res = find_element(app_name, search_name, role, exact, path, ref_id, states)
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
    parser.add_argument("command", choices=["list_apps", "get_tree", "find", "click", "act", "wait"])
    parser.add_argument("--app", help="Application name")
    parser.add_argument("--name", help="Element name")
    parser.add_argument("--role", help="Element role")
    parser.add_argument("--path", help="Element child-index path such as 0/2/1")
    parser.add_argument("--ref", help="Semantic reference id such as ref_0_2_1")
    parser.add_argument("--state", action="append", default=[], help="Required state filter, repeat or pass comma-separated values")
    parser.add_argument("--action", help="Explicit action name to invoke")
    parser.add_argument("--exact", action="store_true", help="Require exact case-insensitive name matching")
    parser.add_argument("--depth", type=int, default=5, help="Max tree depth")
    parser.add_argument("--timeout", type=int, default=5, help="Timeout in seconds")
    args = parser.parse_args()

    states = parse_states(args.state)

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
        if not args.name and args.path is None and not args.ref:
            print(json.dumps({"matched": False, "error": "--name, --path, or --ref is required"}))
            return
        print(json.dumps(find_element(args.app, args.name, args.role, args.exact, args.path, args.ref, states)))
        return

    if args.command == "click":
        if not args.name and args.path is None and not args.ref:
            print(json.dumps({"matched": False, "clicked": False, "error": "--name, --path, or --ref is required"}))
            return
        print(json.dumps(click_element(args.app, args.name, args.role, args.exact, args.path, args.ref, states)))
        return

    if args.command == "act":
        if not args.name and args.path is None and not args.ref:
            print(json.dumps({"matched": False, "invoked": False, "error": "--name, --path, or --ref is required"}))
            return
        print(json.dumps(invoke_element_action(args.app, args.name, args.role, args.exact, args.path, args.ref, states, args.action)))
        return

    if args.command == "wait":
        if not args.name and args.path is None and not args.ref:
            print(json.dumps({"matched": False, "error": "--name, --path, or --ref is required"}))
            return
        print(json.dumps(wait_for_element(args.app, args.name, args.role, args.exact, args.path, args.ref, states, args.timeout)))
        return


if __name__ == "__main__":
    main()
