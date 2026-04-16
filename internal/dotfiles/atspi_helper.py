#!/usr/bin/env python3
import sys
import json
import argparse
import time

try:
    import pyatspi
except ImportError:
    print(json.dumps({"error": "pyatspi not found. Please install python-pyatspi or equivalent."}))
    sys.exit(1)

def element_to_dict(obj, depth=0, max_depth=5):
    if depth > max_depth:
        return {"name": obj.name, "role": obj.getRoleName(), "depth_limit": True}
    
    res = {
        "name": obj.name,
        "role": obj.getRoleName(),
        "description": obj.description,
    }
    
    try:
        states = obj.getState().get_states()
        res["states"] = [pyatspi.state_to_name(s) for s in states]
    except:
        pass

    if depth < max_depth:
        children = []
        for i in range(obj.childCount):
            child = obj.getChildAtIndex(i)
            if child:
                children.append(element_to_dict(child, depth + 1, max_depth))
        res["children"] = children
    
    return res

def list_apps():
    reg = pyatspi.Registry
    apps = []
    for i in range(reg.getAppCount()):
        app = reg.getApp(i)
        apps.append({"name": app.name, "id": i})
    return apps

def get_tree(app_name, max_depth=5):
    reg = pyatspi.Registry
    for i in range(reg.getAppCount()):
        app = reg.getApp(i)
        if app.name == app_name:
            return element_to_dict(app, max_depth=max_depth)
    return {"error": f"App {app_name} not found"}

def find_element(app_name, search_name, role=None):
    reg = pyatspi.Registry
    target_app = None
    for i in range(reg.getAppCount()):
        app = reg.getApp(i)
        if app.name == app_name:
            target_app = app
            break
    
    if not target_app:
        return {"error": f"App {app_name} not found"}

    def search(obj):
        if obj.name == search_name:
            if role is None or obj.getRoleName() == role:
                return obj
        for i in range(obj.childCount):
            res = search(obj.getChildAtIndex(i))
            if res:
                return res
        return None

    found = search(target_app)
    if found:
        return element_to_dict(found, max_depth=0)
    return {"error": f"Element {search_name} not found in {app_name}"}

def click_element(app_name, search_name, role=None):
    reg = pyatspi.Registry
    target_app = None
    for i in range(reg.getAppCount()):
        app = reg.getApp(i)
        if app.name == app_name:
            target_app = app
            break
    
    if not target_app:
        return {"error": f"App {app_name} not found"}

    def search(obj):
        if obj.name == search_name:
            if role is None or obj.getRoleName() == role:
                return obj
        for i in range(obj.childCount):
            res = search(obj.getChildAtIndex(i))
            if res:
                return res
        return None

    found = search(target_app)
    if found:
        try:
            action = found.queryAction()
            if action.nActions > 0:
                action.doAction(0)
                return {"status": "clicked", "name": found.name}
            else:
                return {"error": "Element has no actions"}
        except Exception as e:
            return {"error": f"Failed to click: {str(e)}"}
    return {"error": f"Element {search_name} not found"}

def wait_for_element(app_name, search_name, role=None, timeout=5):
    start = time.time()
    while time.time() - start < timeout:
        res = find_element(app_name, search_name, role)
        if "error" not in res:
            return res
        time.sleep(0.5)
    return {"error": f"Timeout waiting for element {search_name} in {app_name}"}

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("command", choices=["list_apps", "get_tree", "find", "click", "wait"])
    parser.add_argument("--app", help="Application name")
    parser.add_argument("--name", help="Element name")
    parser.add_argument("--role", help="Element role")
    parser.add_argument("--depth", type=int, default=5, help="Max tree depth")
    parser.add_argument("--timeout", type=int, default=5, help="Timeout in seconds")
    
    args = parser.parse_args()
    
    if args.command == "list_apps":
        print(json.dumps(list_apps()))
    elif args.command == "get_tree":
        if not args.app:
            print(json.dumps({"error": "--app required"}))
            return
        print(json.dumps(get_tree(args.app, args.depth)))
    elif args.command == "find":
        if not args.app or not args.name:
            print(json.dumps({"error": "--app and --name required"}))
            return
        print(json.dumps(find_element(args.app, args.name, args.role)))
    elif args.command == "click":
        if not args.app or not args.name:
            print(json.dumps({"error": "--app and --name required"}))
            return
        print(json.dumps(click_element(args.app, args.name, args.role)))
    elif args.command == "wait":
        if not args.app or not args.name:
            print(json.dumps({"error": "--app and --name required"}))
            return
        print(json.dumps(wait_for_element(args.app, args.name, args.role, args.timeout)))

if __name__ == "__main__":
    main()
