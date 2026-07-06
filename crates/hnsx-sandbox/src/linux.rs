#![allow(dead_code)]

// TODO(skeleton): implement Sandbox on Linux using nix + landlock +
// seccompiler + cgroups-rs. See Initial_Architectrue.md §4.2.
//
// Expected submodules once implementation lands:
//   - namespace.rs   (unshare, mount namespaces)
//   - landlock.rs    (filesystem access rules)
//   - seccomp.rs     (syscall filtering)
//   - cgroup.rs      (cpu / memory quotas)
