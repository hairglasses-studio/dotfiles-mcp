#!/usr/bin/env python3
import argparse
import json
import sys
import time

try:
    import pyatspi
except ImportError:
    print(json.dumps({"error": "pyatspi not found. Please install python-atspi (Arch), python-pyatspi, or equivalent."}))
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


def safe_bounds(obj):
    try:
        component = obj.queryComponent()
        extents = component.getExtents(pyatspi.DESKTOP_COORDS)
        return {
            "x": int(extents.x),
            "y": int(extents.y),
            "width": int(extents.width),
            "height": int(extents.height),
        }
    except Exception:
        return None


def safe_attributes(obj):
    out = {}
    try:
        attrs = obj.getAttributes() or []
    except Exception:
        return out
    for raw in attrs:
        if raw is None:
            continue
        text = str(raw).strip()
        if not text:
            continue
        for separator in (":", "="):
            if separator in text:
                key, value = text.split(separator, 1)
                key = key.strip()
                if key:
                    out[key] = value.strip()
                break
        else:
            out[text] = ""
    return out


def safe_relation_type_name(rel):
    try:
        return pyatspi.relation_to_name(rel.getRelationType())
    except Exception:
        pass
    try:
        return str(rel.getRelationType())
    except Exception:
        return "unknown"


def safe_relations(obj):
    out = {}
    try:
        relation_set = obj.getRelationSet()
    except Exception:
        return out
    if relation_set is None:
        return out
    for rel in relation_set:
        try:
            rel_name = safe_relation_type_name(rel)
            if not rel_name:
                continue
            targets = []
            for idx in range(rel.getNTargets()):
                target = rel.getTarget(idx)
                if target is None:
                    continue
                name = (getattr(target, "name", None) or "").strip()
                if name:
                    targets.append(name)
            if targets:
                out[rel_name] = targets
        except Exception:
            continue
    return out


def safe_text_interface(obj):
    try:
        return obj.queryText()
    except Exception:
        return None


def safe_text_length(obj):
    text_iface = safe_text_interface(obj)
    if text_iface is None:
        return 0
    try:
        count = getattr(text_iface, "characterCount", 0)
        if count is None or count < 0:
            return 0
        return int(count)
    except Exception:
        return 0


def safe_text_value(obj):
    text_iface = safe_text_interface(obj)
    if text_iface is None:
        return None
    for end in (safe_text_length(obj), -1):
        try:
            return text_iface.getText(0, end)
        except Exception:
            continue
    return None


def safe_numeric_value(obj):
    try:
        value_iface = obj.queryValue()
        return value_iface.currentValue
    except Exception:
        return None


def safe_value_fields(obj):
    res = {}
    text_value = safe_text_value(obj)
    if text_value is not None:
        res["text"] = text_value
        res["value"] = text_value
        res["value_kind"] = "text"

    numeric_value = safe_numeric_value(obj)
    if numeric_value is not None:
        res["value"] = numeric_value
        res["value_kind"] = "numeric"
    return res


def app_info(idx, app):
    return {"id": idx, "name": app.name, "role": safe_role(app)}


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
            "child_count": getattr(obj, "childCount", 0),
            "depth_limit": True,
        }

    res = {
        "path": path,
        "ref": path_to_ref(path),
        "name": obj.name,
        "role": safe_role(obj),
        "description": getattr(obj, "description", None),
        "child_count": getattr(obj, "childCount", 0),
    }

    states = safe_states(obj)
    if states:
        res["states"] = states

    actions = safe_actions(obj)
    if actions:
        res["actions"] = actions

    bounds = safe_bounds(obj)
    if bounds is not None:
        res["bounds"] = bounds

    attributes = safe_attributes(obj)
    if attributes:
        res["attributes"] = attributes

    relations = safe_relations(obj)
    if relations:
        res["relations"] = relations

    res.update(safe_value_fields(obj))

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


def list_windows(app_name=""):
    windows = []
    for idx, app in iter_apps():
        if app_name and not match_value(app.name, app_name, exact=False):
            continue
        for child_idx in range(getattr(app, "childCount", 0)):
            child = app.getChildAtIndex(child_idx)
            if child is None:
                continue
            path = str(child_idx)
            info = element_to_dict(child, path, 0, 0)
            info["app"] = app_info(idx, app)
            windows.append(info)
    return {"windows": windows}


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


def search_elements(root, search_name, role=None, exact=False, path=None, ref_id=None, states=None, limit=20):
    if ref_id or path is not None:
        found, found_path = search_element(root, search_name, role, exact, path, ref_id, states)
        if found is None:
            return []
        return [(found, found_path or "")]

    results = []

    def visit(obj, current_path=""):
        if len(results) >= limit:
            return
        if match_value(obj.name, search_name, exact) and (
            role is None or match_value(safe_role(obj), role, exact=True)
        ) and state_matches(obj, states):
            results.append((obj, current_path))
        for idx in range(getattr(obj, "childCount", 0)):
            child = obj.getChildAtIndex(idx)
            if child is None:
                continue
            child_path = str(idx) if current_path == "" else f"{current_path}/{idx}"
            visit(child, child_path)

    visit(root, "")
    return results


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


def invoke_found_action(idx, app, found, found_path, requested_action=None):
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
        "app": app_info(idx, app),
        "element": element_to_dict(found, found_path or "", 0, 0),
    }


def focus_found_element(idx, app, found, found_path):
    last_error = None
    try:
        component = found.queryComponent()
        component.grabFocus()
        return {
            "matched": True,
            "focused": True,
            "app": app_info(idx, app),
            "element": element_to_dict(found, found_path or "", 0, 0),
        }
    except Exception as exc:
        last_error = exc

    for requested_action in ("grab focus", "focus"):
        result = invoke_found_action(idx, app, found, found_path, requested_action)
        if result.get("invoked"):
            result["focused"] = True
            result["clicked"] = False
            return result
        if result.get("error"):
            last_error = result["error"]

    return {
        "matched": True,
        "focused": False,
        "error": f"Failed to focus element: {last_error}",
        "element": element_to_dict(found, found_path or "", 0, 0),
    }


def set_found_text(idx, app, found, found_path, text):
    try:
        editable = found.queryEditableText()
    except Exception as exc:
        return {
            "matched": True,
            "updated": False,
            "error": f"Element has no editable text interface: {exc}",
            "element": element_to_dict(found, found_path or "", 0, 0),
        }

    last_error = None
    for candidate in (text, text.encode("utf-8")):
        try:
            editable.setTextContents(candidate)
            element = element_to_dict(found, found_path or "", 0, 0)
            return {
                "matched": True,
                "updated": True,
                "app": app_info(idx, app),
                "value": element.get("value"),
                "value_kind": element.get("value_kind"),
                "element": element,
            }
        except Exception as exc:
            last_error = exc

    count = safe_text_length(found)
    for candidate in (text, text.encode("utf-8")):
        try:
            try:
                editable.deleteText(0, count)
            except Exception:
                pass
            editable.insertText(0, candidate, len(text))
            element = element_to_dict(found, found_path or "", 0, 0)
            return {
                "matched": True,
                "updated": True,
                "app": app_info(idx, app),
                "value": element.get("value"),
                "value_kind": element.get("value_kind"),
                "element": element,
            }
        except Exception as exc:
            last_error = exc

    return {
        "matched": True,
        "updated": False,
        "error": f"Failed to set text: {last_error}",
        "element": element_to_dict(found, found_path or "", 0, 0),
    }


def set_found_value(idx, app, found, found_path, value):
    try:
        value_iface = found.queryValue()
        value_iface.currentValue = float(value)
    except Exception as exc:
        return {
            "matched": True,
            "updated": False,
            "error": f"Element has no writable numeric value interface: {exc}",
            "element": element_to_dict(found, found_path or "", 0, 0),
        }

    element = element_to_dict(found, found_path or "", 0, 0)
    return {
        "matched": True,
        "updated": True,
        "app": app_info(idx, app),
        "value": element.get("value"),
        "value_kind": element.get("value_kind"),
        "element": element,
    }


def read_found_value(idx, app, found, found_path):
    element = element_to_dict(found, found_path or "", 0, 0)
    if "value_kind" not in element:
        return {
            "matched": True,
            "error": "Element has no readable text or value interface",
            "element": element,
        }

    return {
        "matched": True,
        "app": app_info(idx, app),
        "value": element.get("value"),
        "value_kind": element.get("value_kind"),
        "element": element,
    }


def get_tree(app_name, max_depth=5):
    idx, app = find_app(app_name)
    if app is None:
        return {"matched": False, "error": f"App {app_name} not found"}
    return {
        "matched": True,
        "app": app_info(idx, app),
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
        "app": app_info(idx, app),
        "element": element_to_dict(found, found_path or "", 0, 0),
    }


def find_all_elements(app_name, search_name, role=None, exact=False, path=None, ref_id=None, states=None, limit=20):
    idx, app = find_app(app_name)
    if app is None:
        return {"matched": False, "count": 0, "error": f"App {app_name} not found"}

    matches = search_elements(app, search_name, role, exact, path, ref_id, states, limit)
    return {
        "matched": len(matches) > 0,
        "count": len(matches),
        "app": app_info(idx, app),
        "elements": [element_to_dict(obj, matched_path or "", 0, 0) for obj, matched_path in matches],
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

    return invoke_found_action(idx, app, found, found_path, requested_action)


def click_element(app_name, search_name, role=None, exact=False, path=None, ref_id=None, states=None):
    return invoke_element_action(app_name, search_name, role, exact, path, ref_id, states)


def focus_element(app_name, search_name, role=None, exact=False, path=None, ref_id=None, states=None):
    idx, app = find_app(app_name)
    if app is None:
        return {"matched": False, "focused": False, "error": f"App {app_name} not found"}

    found, found_path = search_element(app, search_name, role, exact, path, ref_id, states)
    if found is None:
        return {"matched": False, "focused": False, "error": f"Element not found in {app_name}"}

    return focus_found_element(idx, app, found, found_path)


def read_element_value(app_name, search_name, role=None, exact=False, path=None, ref_id=None, states=None):
    idx, app = find_app(app_name)
    if app is None:
        return {"matched": False, "error": f"App {app_name} not found"}

    found, found_path = search_element(app, search_name, role, exact, path, ref_id, states)
    if found is None:
        return {"matched": False, "error": f"Element not found in {app_name}"}

    return read_found_value(idx, app, found, found_path)


def set_element_text(app_name, search_name, text, role=None, exact=False, path=None, ref_id=None, states=None):
    idx, app = find_app(app_name)
    if app is None:
        return {"matched": False, "updated": False, "error": f"App {app_name} not found"}

    found, found_path = search_element(app, search_name, role, exact, path, ref_id, states)
    if found is None:
        return {"matched": False, "updated": False, "error": f"Element not found in {app_name}"}

    return set_found_text(idx, app, found, found_path, text)


def set_element_value(app_name, search_name, value, role=None, exact=False, path=None, ref_id=None, states=None):
    idx, app = find_app(app_name)
    if app is None:
        return {"matched": False, "updated": False, "error": f"App {app_name} not found"}

    found, found_path = search_element(app, search_name, role, exact, path, ref_id, states)
    if found is None:
        return {"matched": False, "updated": False, "error": f"Element not found in {app_name}"}

    return set_found_value(idx, app, found, found_path, value)


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
    parser.add_argument(
        "command",
        choices=["list_apps", "list_windows", "get_tree", "find", "find_all", "click", "act", "focus", "read_value", "set_text", "set_value", "wait"],
    )
    parser.add_argument("--app", help="Application name")
    parser.add_argument("--name", help="Element name")
    parser.add_argument("--role", help="Element role")
    parser.add_argument("--path", help="Element child-index path such as 0/2/1")
    parser.add_argument("--ref", help="Semantic reference id such as ref_0_2_1")
    parser.add_argument("--state", action="append", default=[], help="Required state filter, repeat or pass comma-separated values")
    parser.add_argument("--action", help="Explicit action name to invoke")
    parser.add_argument("--text", default="", help="Text to set for editable elements")
    parser.add_argument("--value", type=float, default=0.0, help="Numeric value to set for sliders or similar widgets")
    parser.add_argument("--exact", action="store_true", help="Require exact case-insensitive name matching")
    parser.add_argument("--limit", type=int, default=20, help="Max matches for find_all")
    parser.add_argument("--depth", type=int, default=5, help="Max tree depth")
    parser.add_argument("--timeout", type=int, default=5, help="Timeout in seconds")
    args = parser.parse_args()

    states = parse_states(args.state)

    if args.command == "list_apps":
        print(json.dumps(list_apps()))
        return

    if args.command == "list_windows":
        print(json.dumps(list_windows(args.app or "")))
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

    if args.command == "find_all":
        if not args.name and args.path is None and not args.ref and not states:
            print(json.dumps({"matched": False, "count": 0, "error": "--name, --path, --ref, or --state is required"}))
            return
        print(json.dumps(find_all_elements(args.app, args.name, args.role, args.exact, args.path, args.ref, states, args.limit)))
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

    if args.command == "focus":
        if not args.name and args.path is None and not args.ref:
            print(json.dumps({"matched": False, "focused": False, "error": "--name, --path, or --ref is required"}))
            return
        print(json.dumps(focus_element(args.app, args.name, args.role, args.exact, args.path, args.ref, states)))
        return

    if args.command == "read_value":
        if not args.name and args.path is None and not args.ref:
            print(json.dumps({"matched": False, "error": "--name, --path, or --ref is required"}))
            return
        print(json.dumps(read_element_value(args.app, args.name, args.role, args.exact, args.path, args.ref, states)))
        return

    if args.command == "set_text":
        if not args.name and args.path is None and not args.ref:
            print(json.dumps({"matched": False, "updated": False, "error": "--name, --path, or --ref is required"}))
            return
        print(json.dumps(set_element_text(args.app, args.name, args.text, args.role, args.exact, args.path, args.ref, states)))
        return

    if args.command == "set_value":
        if not args.name and args.path is None and not args.ref:
            print(json.dumps({"matched": False, "updated": False, "error": "--name, --path, or --ref is required"}))
            return
        print(json.dumps(set_element_value(args.app, args.name, args.value, args.role, args.exact, args.path, args.ref, states)))
        return

    if args.command == "wait":
        if not args.name and args.path is None and not args.ref:
            print(json.dumps({"matched": False, "error": "--name, --path, or --ref is required"}))
            return
        print(json.dumps(wait_for_element(args.app, args.name, args.role, args.exact, args.path, args.ref, states, args.timeout)))
        return


if __name__ == "__main__":
    main()
