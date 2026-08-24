# deploy-kit: design, trade-offs, and non-goals

Status: accepted
Author: Parag Sawant

Why deploy-kit is built the way it is - and, more than any other repo in this set, an
honest statement of what it is and isn't. It's a scaffold with a real interface and a
secure-by-default posture, not a production deployer, and the design is about making
that boundary crisp.

## Problem and goals

Deploying an LLM app into "some cloud" means relearning a different set of primitives
every time - and it's easy to ship something insecure by default because the safe
options are opt-in. deploy-kit is one command with a consistent interface across
targets that prints a plan with secure defaults baked in. Goals:

1. **One interface, many targets** - `plan` / `up` / `down` against `local`, `azure`,
   or `aws`, so the mental model is the same regardless of where you deploy.
2. **Secure by default** - the generated plan defaults to the locked-down choices
   (no public exposure unless asked, secrets from a vault not the environment, least
   privilege), so the easy path is the safe path.
3. **A target interface that's real even though the apply is a scaffold** - the shape is
   production-ready; the destructive calls are stubbed.

## Key design decision: a scaffold with a real seam, stated plainly

This is the most important thing to be honest about. deploy-kit **prints the plan** and
runs the secure-by-default checks; it does not actually call cloud APIs to create or
destroy infrastructure. That's deliberate - a portfolio tool that really provisions
cloud resources is a liability, not an asset - but it would be dishonest to present it as
a working deployer. So the design puts the *interface* front and center: each target
implements the same contract, the CLI drives them uniformly, and the place where real
`apply`/`destroy` calls would go is a clearly marked seam. You could wire a real backend
behind it without changing the surface.

I'd rather ship a clean interface and say "the apply is a stub" than fake a deployment.

## Trade-offs I made on purpose

- **Plan/print, not apply.** See above. The value is the consistent interface and the
  secure defaults; the actual cloud mutation is intentionally not implemented, and the
  README and this doc say so rather than implying otherwise.
- **Secure-by-default over configurable-by-default.** The plan starts from the locked
  choices and makes you opt into exposure, which is the opposite of most quickstarts.
  It's slightly more friction for a demo and much safer as a habit - the right default
  for anything that touches deployment.
- **A small set of targets with IaC stubs.** `local`, `azure`, `aws` cover the common
  shapes (a box, two major clouds) without pretending to support everything. The IaC
  files are stubs that show structure, not runnable modules - flagged as such.

## Why there's no benchmark or stress suite

deploy-kit doesn't have a hot path, a data structure, or a failure mode to fuzz - it
composes a plan and prints it. A throughput number or a chaos suite would be theater.
What matters is that the interface is consistent across targets and the defaults are
secure, and the tests cover exactly that: each target produces a well-formed plan, and
the secure-by-default checks fire.

## Non-goals

- **Not a real provisioner.** It does not create, modify, or destroy cloud resources.
  The apply/destroy path is a stub behind a real interface; wiring a backend
  (Terraform, Bicep, cloud SDKs) is the production step.
- **Not a secrets manager or an IAM tool.** It defaults to referencing a vault and to
  least privilege; it doesn't implement either.
- **Not a full IaC framework.** The `iac/` files are structural stubs, not runnable
  infrastructure modules.

Part of [parag-labs](https://github.com/parag-labs) - small, focused tools for building AI systems you can trust.
