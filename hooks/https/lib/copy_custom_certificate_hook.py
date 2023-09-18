from deckhouse import hook
from . import module

CUSTOM_CERTIFICATES = "custom_certificates"

config = f"""
configVersion: v1
beforeHelm: 10
kubernetes:
- name: "{CUSTOM_CERTIFICATES}"
  keepFullObjectsInMemory: false
  waitForSynchronization: false
  apiVersion: v1
  kind: Secret
  namespace:
    nameSelector:
      matchNames: ["d8-system"] 
  labelSelector:
    matchExpressions:
    - key: "owner"
      operator: "NotIn"
      values: ["helm"]     
  jqFilter: |
    {{"name":  .metadata.name, "data": .data}}  
"""

def register_hook(name: str):
    global module_name
    module_name = name
    hook.run(copy_custom_certificates_handler, config=config)

def copy_custom_certificates_handler(ctx: hook.Context):  
    custom_certificates = {}
    for s in ctx.snapshots.get(CUSTOM_CERTIFICATES, []):
        if (filtered := s.get("filterResult")) is not None:
          custom_certificates[filtered["name"]] = filtered["data"]
    if len(custom_certificates) == 0:
        print("No custom certificates received, skipping setting values")
        return

    https_mode = module.get_https_mode(module_name=module_name,
                                       values=ctx.values)
    if https_mode != "CustomCertificate":
        data = ctx.values[module_name]["internal"].get("customCertificateData")
        if data is not None:
          ctx.values[module_name]["internal"].pop("customCertificateData")
          ctx.values_patches.update(updated_values=ctx.values)
        return

    raw_secret_name = module.get_values_first_defined(ctx.values, 
                                                      f"$.{module_name}.https.customCertificate.secretName",
                                                      "$.global.modules.https.customCertificate.secretName")
    if raw_secret_name is None:
        return
    secret_name = str(raw_secret_name)
    if secret_name == "":
      return 
		
    secret_data = custom_certificates.get(secret_name)
    if secret_data is None:
        print("custom certificate secret name is configured, but secret with this name doesn't exist")
        return
    ctx.values[module_name]["internal"]["customCertificateData"] = secret_data
    ctx.values_patches.update(updated_values=ctx.values)
