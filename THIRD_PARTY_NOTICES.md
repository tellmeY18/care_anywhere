# CARE Clinic reuse

`internal/atomicfile` adapts CARE Clinic's atomic replacement and Windows private
file ACL implementation from `app/internal/sys/atomicfile`.
`internal/diskspace` adapts its nearest-existing-directory and filesystem capacity
checks. `nix/buckets.py` ports its bucket initialization and public facility download
policy from `deployments/minio/entrypoint.sh` to the native Python runtime.
Source: https://github.com/ohcnetwork/care_clinic (main, retrieved 2026-10-04).
The original API and durability ordering are retained, with platform files combined.

The operation lock, staged restore-before-activation model, separate recovery
material, and prohibition on replacing live data follow CARE Clinic's architecture.
Docker-specific backup helpers cannot be reused directly for a VM disk format.
Existing Clinic backups are not interchangeable with this preview's appliance
backup format.

## CARE Clinic MIT License

Copyright (c) 2026 Open Healthcare Network Foundation

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

Other components retain their own licenses. Distributing appliance images or
QEMU/Firecracker helpers requires their corresponding notices and source-offer
obligations, independent of the launcher license.

# CARE Clinic desktop frontend and CARE Onboarding

`frontend/` is adapted from `ohcnetwork/care_clinic` (MIT), source snapshot
`de9d492407c6004e7aae33d5f40c9d6459c65fea`. The React components, setup layout,
administrator form, control panel, backup list, storage views, typography and
styles are reused. The Wails bridge is adapted to the loopback VM control API;
container-only and unavailable alpha operations are not exposed.

CARE Onboarding is built without source modifications from
`ohcnetwork/care_onboarding_fe` commit
`d78c2f177d281f8b27370e8c9f04571a70fcba08` (MIT) and hosted locally in the package.
Its source, data provenance, and license are at
https://github.com/ohcnetwork/care_onboarding_fe/tree/d78c2f177d281f8b27370e8c9f04571a70fcba08.
