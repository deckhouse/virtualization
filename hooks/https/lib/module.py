from jsonpath_ng.ext import parse

def get_values_first_defined(values: dict, *keys):
    return _get_first_defined(values, keys)


def _get_first_defined(values: dict, keys: tuple):
    for i in range(len(keys)):
        jsonpath_expression = parse(keys[i])
        for match in jsonpath_expression.find(values):
             return match.value
    return


def get_https_mode(module_name: str, values: dict) -> str:
    module_path = f"$.{module_name}.https.mode"
    global_path = "$.global.modules.https.mode"
    https_mode = get_values_first_defined(values, module_path, global_path)
    if https_mode is not None:
        return str(https_mode)
    raise Exception("https mode is not defined")
