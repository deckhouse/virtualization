# Nehalem HT removal summary

## Problem

Need to check whether the QEMU `Nehalem` CPU model exposes the HT/Hyper-Threading CPUID feature and remove it.

## Findings

Relevant file:
- `target/i386/cpu.c`

What was found:
- The `Nehalem` CPU model is defined in `target/i386/cpu.c`.
- `CPUID_HT` is not explicitly present in the `Nehalem` model's base `.features[FEAT_1_EDX]` initializer.
- But QEMU later auto-enables `CPUID_HT` when topology indicates more than one thread per package:

```c
if (x86_threads_per_pkg(&env->topo_info) > 1) {
    env->features[FEAT_1_EDX] |= CPUID_HT;
}
```

So in practice `Nehalem` can still advertise HT.

## First attempt and issue

Initial change was to add this directly to the `Nehalem` definition:

```c
.filtered_features[FEAT_1_EDX] = CPUID_HT,
```

That did not compile on this QEMU version because `X86CPUDefinition` in this tree did not contain a `filtered_features` field.

Build error:
- `X86CPUDefinition has no member named filtered_features`

## What was changed

Changes were made in `target/i386/cpu.c`:

### 1. Extend `X86CPUDefinition`
Added a model-level filtered feature array:

```c
FeatureWordArray filtered_features;
```

### 2. Apply model-level filtered features when loading the CPU model
Updated `x86_cpu_load_model()` from:

```c
for (w = 0; w < FEATURE_WORDS; w++) {
    env->features[w] = def->features[w];
}
```

to:

```c
for (w = 0; w < FEATURE_WORDS; w++) {
    env->features[w] = def->features[w];
    cpu->filtered_features[w] |= def->filtered_features[w];
}
```

### 3. Mask HT for `Nehalem`
Added to the `Nehalem` CPU definition:

```c
.filtered_features[FEAT_1_EDX] = CPUID_HT,
```

## Expected result

`Nehalem` should no longer expose the HT CPUID feature, even if topology-based logic would otherwise auto-enable it.

## Patch

Patch file written to:
- `/Users/max/kode/virtualization/images/qemu/patches/remove-nehalem-ht.patch`

## Validation

Tried to run a local compile check:

```bash
ninja -C build libqemu-x86_64-softmmu.a.p/target_i386_cpu.c.o
```

But it failed because there is no local build directory in this checkout:
- `/Users/max/kode/qemu`

So the patch and source changes were updated, but compilation was not verified locally here.
