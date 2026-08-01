# Support reporter security boundary

The `reporter` role is reserved for the future Support surface. It is not a
general workspace member. Until the dedicated `/api/support/*` allowlist is
implemented, reporters are denied from the existing generic workspace-scoped
route groups, daemon routes authenticated with a user PAT/JWT, realtime
subscriptions, and the legacy workspace-scoped upload and authenticated
attachment-download paths. The two deprecated onboarding bootstrap shims also
explicitly reject reporters after loading their membership.

This is not a blanket claim about every API: user-scoped and public endpoints
are outside this perimeter and must not expose Support data without their own
authorization review.

The Support surface must remain part of Multica. It may be named `Suporte dos
Sistemas Innova`, but must preserve the Multica name, logo, and required
attribution in every derived UI.

This repository is not a production deployment. Adding the role does not
authorize invitations, production migrations, or release automation.
