# Deploying the Jenkins Acceptance-Test Setup (ssp)

One-time runbook for wiring `ci/jenkins/Jenkinsfile.acceptance` and
`.github/workflows/acceptance-tests.yml` up to a real Jenkins instance and a
real SSP lab. Confirmed so far: the target Jenkins is the CIVT node at
<https://jenkins-nsxqe.devops.broadcom.net/>, and it has **no existing
network path to the SSP/SSPI labs** (it's used for NSX-QE today).

For the companion SSPI appliance provider's runbook, see
[`terraform-provider-sspi`'s `ci/jenkins/DEPLOYMENT.md`](https://github.com/vmware/terraform-provider-sspi/blob/main/ci/jenkins/DEPLOYMENT.md)
-- the two are done together in practice since they share the Jenkins
instance and often the same jump host.

## Phase 1: Get network access to the lab

Check first whether a jump host for the SSP Nimbus testbeds already exists
-- `TESTING.md` documents `ssh worker@<jump_host_ip> -D 9999 -N` for manual
testing today. If one exists, reuse it and skip to Phase 2. If not:

```bash
# On a new small VM inside the SSP lab network segment (routable to the
# Nimbus testbeds):
sudo apt-get update && sudo apt-get install -y openssh-server
sudo useradd -m -s /bin/bash jenkins-tunnel
sudo -u jenkins-tunnel mkdir -p /home/jenkins-tunnel/.ssh

# Generate a dedicated keypair for Jenkins (on your workstation, not the jump host):
ssh-keygen -t ed25519 -f jenkins_lab_tunnel_key -N "" -C "jenkins-nsxqe-lab-tunnel"

# Install the PUBLIC key, restricted to port-forwarding only (no shell):
echo 'no-pty,no-agent-forwarding,no-X11-forwarding,command="echo tunnel-only account" ssh-ed25519 AAAA...' \
  | sudo -u jenkins-tunnel tee /home/jenkins-tunnel/.ssh/authorized_keys
sudo chmod 700 /home/jenkins-tunnel/.ssh && sudo chmod 600 /home/jenkins-tunnel/.ssh/authorized_keys
# Confirm sshd_config allows forwarding (`AllowTcpForwarding yes`, the default).
```

Keep `jenkins_lab_tunnel_key` (private half) for Phase 3.

## Phase 2: Point a Jenkins agent at it

Dynamic SSH port-forwarding (`-D`, what the Jenkinsfile's "Open lab SOCKS5
tunnel" stage already does) only needs the **agent** to reach the jump host
over SSH -- it does not need to be inside the lab network itself. Try the
existing agent first:

```bash
# On the existing jenkins-nsxqe agent:
ssh -o BatchMode=yes jenkins-tunnel@<jump_host_ip> exit && echo "reachable"
go version   # must satisfy go.mod's `go 1.25.13`
jq --version
```

If reachable with the right tools: **Manage Jenkins -> Nodes -> (agent) ->
Configure -> Labels -> add `ssp-lab`**. Done, no new VM needed.

If unreachable (firewalled) or missing tools, provision a new agent:

```bash
sudo apt-get install -y openjdk-17-jre-headless git jq openssh-client curl
curl -LO https://go.dev/dl/go1.25.1.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.25.1.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' | sudo tee -a /etc/profile.d/go.sh
```

Then **Manage Jenkins -> Nodes -> New Node**: name `ssp-lab-agent-1`,
Permanent Agent, Labels `ssp-lab`, Launch method "Launch agents via SSH",
Host = this VM's IP, credentials = a new SSH credential for Jenkins-to-VM
login (separate from the jump-host key above).

## Phase 3: Create the Jenkins credentials

Manual, via **Manage Jenkins -> Credentials -> System -> Global
credentials -> Add Credentials** (don't script secret values into a file):

| ID | Kind | Value source |
| --- | --- | --- |
| `lab-jumphost-ssh` | SSH Username with private key | username `jenkins-tunnel`, paste the Phase 1 private key |
| `ssp-acc-host` / `-username` / `-password` / `-insecure` | Secret text | from whoever manages the SSP Nimbus testbed |
| `nsx-acc-host` / `-username` / `-password` / `-certificate` | Secret text | from whoever manages the NSX Manager lab (needed only for `ssp_site` tests) |
| `github-checks-token` | Secret text | GitHub.com -> Settings -> Developer settings -> Fine-grained tokens -> New -> repo access: `terraform-provider-ssp` -> permission: **Checks: Read and write** |

## Phase 4: Create the Pipeline job

**New Item -> Pipeline**, name `ssp-acceptance-tests`:
Pipeline -> Definition: "Pipeline script from SCM" -> SCM: Git ->
Repository URL `https://github.com/vmware/terraform-provider-ssp.git` ->
Script Path `ci/jenkins/Jenkinsfile.acceptance` -> Save.

## Phase 5: Wire GitHub -> Jenkins

```bash
# Generate the Jenkins API token first: Jenkins -> your user avatar ->
# Configure -> API Token -> Add new Token.
gh secret set JENKINS_USER --repo vmware/terraform-provider-ssp
gh secret set JENKINS_API_TOKEN --repo vmware/terraform-provider-ssp
```

`JENKINS_URL` is already defaulted to the CIVT node in
`.github/workflows/acceptance-tests.yml` -- only set the `JENKINS_URL` repo
*variable* if overriding it to a different instance later.

## Phase 6: End-to-end smoke test

1. Open a small, same-repo (not fork) test PR.
2. Comment `/run-acceptance-tests -run=TestAccSiteDataSource` (cheap,
   read-only) as a maintainer-association account.
3. Confirm: the "Acceptance Tests (Jenkins)" GitHub Actions job fires ->
   the Jenkins job starts -> the SOCKS5 tunnel opens -> `make testacc-ci`
   runs -> a "Jenkins Acceptance Tests" Check appears on the PR and
   resolves to success/failure correctly.
