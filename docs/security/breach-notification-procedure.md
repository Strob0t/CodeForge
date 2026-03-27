# Data Breach Notification Procedure

## Scope

This procedure applies to all personal data breaches in CodeForge deployments. A "personal data breach" is any security incident leading to the accidental or unlawful destruction, loss, alteration, unauthorized disclosure of, or access to, personal data transmitted, stored, or otherwise processed (GDPR Art. 4(12)).

All team members who become aware of a suspected breach must follow this procedure immediately, regardless of the perceived severity. When in doubt, escalate -- do not self-assess.

---

## 1. Detect and Contain (0-4 hours)

**Objective:** Stop the breach from worsening and preserve evidence.

### Detection Signals

| Signal | Source | Example |
|---|---|---|
| Mass failed logins | Auth logs, Prometheus alerts | >10 failures/sec sustained for 2+ minutes |
| Unauthorized data access | Audit log, database query logs | Bulk SELECT on `users`/`conversations` from unexpected source |
| Credential stuffing | Auth middleware, rate limiter | Distributed login attempts across many accounts |
| Data exfiltration | Network monitoring, API logs | Unusually large data exports, bulk API calls to `/me/export` |
| Unauthorized API key usage | API key audit trail | Key used from unexpected IP or after revocation |
| Anomalous agent behavior | Agent execution logs | Agent accessing files outside workspace, unexpected network calls |

### Containment Actions

1. **Isolate affected systems** -- disable compromised accounts, revoke API keys, block suspicious IPs
2. **Preserve evidence** -- snapshot logs, database state, and container state before any remediation
3. **Notify the incident lead** -- designated DPO or security lead
4. **Activate the incident channel** -- create a dedicated communication channel for the response team
5. **Document the timeline** -- record when the breach was detected, by whom, and initial observations

---

## 2. Assess Risk (4-24 hours)

**Objective:** Determine the nature, scope, and likely impact of the breach.

### Assessment Checklist

- [ ] **What data was affected?** -- categories (names, emails, API keys, conversation content, project metadata)
- [ ] **How many data subjects are affected?** -- approximate count of users/organizations
- [ ] **What was the cause?** -- vulnerability, misconfiguration, insider threat, social engineering
- [ ] **Is the breach ongoing?** -- confirm containment is effective
- [ ] **What is the likely impact?** -- identity theft, financial loss, reputational damage, loss of confidentiality
- [ ] **Are there cross-border implications?** -- data subjects in multiple jurisdictions

### Risk Classification

| Level | Criteria | Action |
|---|---|---|
| **Low** | No sensitive personal data, small scope, quickly contained | Document only (no SA notification required) |
| **Medium** | Personal data exposed but low likelihood of harm | Notify Supervisory Authority within 72 hours |
| **High** | Sensitive data (credentials, financial, health), large scale, or ongoing | Notify SA within 72 hours AND notify affected data subjects |
| **Critical** | Active exploitation, mass credential compromise, public exposure | Notify SA immediately, notify subjects without undue delay, engage legal counsel |

A breach that is "unlikely to result in a risk to the rights and freedoms of natural persons" does not require SA notification (Art. 33(1) exemption), but MUST still be documented internally.

---

## 3. Notify Supervisory Authority (by 72 hours, per GDPR Art. 33)

**Deadline:** Within 72 hours of becoming aware of the breach. If notification is delayed beyond 72 hours, provide reasons for the delay (Art. 33(1)).

**Phased reporting:** If not all information is available within 72 hours, submit an initial notification and provide additional details in phases as they become available (Art. 33(4)).

### SA Notification Template

The following fields are required per GDPR Art. 33(3):

```
SUPERVISORY AUTHORITY BREACH NOTIFICATION
==========================================

1. Nature of the breach
   Description: [What happened -- unauthorized access/data loss/etc.]
   Date & time detected: [YYYY-MM-DD HH:MM UTC]
   Date & time occurred (if known): [YYYY-MM-DD HH:MM UTC]
   Duration: [How long the breach persisted]

2. Categories of personal data affected
   [ ] Names / contact details
   [ ] Email addresses
   [ ] Authentication credentials (hashed/plaintext)
   [ ] API keys / tokens
   [ ] Conversation content (may contain user-submitted code/data)
   [ ] Project metadata
   [ ] Usage/billing data
   [ ] Other: [specify]

3. Categories of data subjects affected
   [ ] Registered users
   [ ] Organization administrators
   [ ] API consumers
   [ ] Other: [specify]

4. Approximate number of data subjects affected
   Count: [number or best estimate]
   Approximate number of records: [number or best estimate]

5. Data Protection Officer (DPO) contact
   Name: [DPO name]
   Email: [DPO email]
   Phone: [DPO phone]

6. Likely consequences of the breach
   [Describe the potential impact on affected individuals --
    e.g., unauthorized access to accounts, identity theft risk,
    exposure of proprietary code, loss of confidentiality]

7. Measures taken or proposed
   a) Measures to address the breach:
      [e.g., credentials rotated, access revoked, vulnerability patched]
   b) Measures to mitigate adverse effects:
      [e.g., affected users notified, monitoring enhanced, support provided]

8. Cross-border processing (if applicable)
   Lead Supervisory Authority: [country]
   Other SAs to be notified: [list]
```

### Submission

- Submit via the relevant Supervisory Authority's online portal (e.g., ICO breach reporting tool for UK, national DPA portals for EU member states)
- Retain a copy of the submission with timestamp
- Assign a tracking reference number

---

## 4. Notify Data Subjects (if high risk, per GDPR Art. 34)

**Trigger:** Notification to data subjects is required when the breach is "likely to result in a high risk to the rights and freedoms of natural persons" (Art. 34(1)).

**Exceptions (Art. 34(3)):** Subject notification is NOT required if:
- (a) Appropriate technical measures were in place (e.g., encryption) rendering data unintelligible
- (b) Subsequent measures ensure the high risk is no longer likely to materialize
- (c) It would involve disproportionate effort -- in which case, a public communication is required instead

### Subject Notification Content

Per Art. 34(2), the notification must include (in clear and plain language):
- Nature of the breach
- DPO contact details
- Likely consequences
- Measures taken and recommended protective actions (e.g., "change your password", "revoke and regenerate API keys")

### Notification Channels

| Channel | When to use |
|---|---|
| Email to registered address | Default for known users |
| In-app notification banner | Supplement to email, for active users |
| Public notice on status page | When individual notification is impractical |

---

## 5. Investigate and Remediate

**Objective:** Identify root cause, close the vulnerability, and prevent recurrence.

### Investigation Steps

1. **Root cause analysis** -- determine the technical and procedural cause
2. **Attack vector reconstruction** -- trace the full path from initial access to data exposure
3. **Scope confirmation** -- verify the assessment from Phase 2; adjust notifications if scope changes
4. **Vulnerability remediation** -- patch, configuration fix, access control update
5. **Credential rotation** -- force password resets and API key regeneration for affected accounts
6. **Enhanced monitoring** -- increase logging verbosity and alert sensitivity for affected systems

### CodeForge-Specific Remediation

| Scenario | Action |
|---|---|
| Compromised API keys | Revoke via admin panel, notify key owners, audit usage history |
| Agent sandbox escape | Disable affected execution mode, review safety layer configuration |
| Database exposure | Rotate database credentials, audit query logs, review network policies |
| LLM provider key leak | Rotate provider keys in LiteLLM config, check for unauthorized usage |
| Unauthorized workspace access | Revoke project access, review audit trail, check branch isolation |

---

## 6. Document and Learn

**Objective:** Maintain a complete breach register and improve defenses.

### Documentation Requirements (Art. 33(5))

ALL breaches must be documented regardless of whether SA notification was required. The record must include:

| Field | Description |
|---|---|
| Breach ID | Unique identifier (e.g., `BREACH-2026-001`) |
| Date detected | When the breach was first identified |
| Date occurred | When the breach actually started (if known) |
| Date contained | When the breach was effectively stopped |
| Description | Facts of the breach |
| Categories of data | What personal data was involved |
| Number of subjects | Approximate count |
| Effects | Actual and potential consequences |
| Remedial measures | Actions taken to address and prevent recurrence |
| SA notified | Yes/No, date, reference number |
| Subjects notified | Yes/No, date, method |
| Decision rationale | Why notification was/was not required |
| Lessons learned | Post-incident improvements identified |

### Post-Incident Review

Within 14 days of breach closure:

1. **Incident report** -- comprehensive written report covering all phases
2. **Lessons learned session** -- team review of what went well and what failed
3. **Policy updates** -- revise security policies, access controls, monitoring rules
4. **Procedure updates** -- update this document if gaps were identified
5. **Training** -- address any human factors that contributed to the breach
6. **Architecture review** -- assess whether structural changes are needed (e.g., additional safety layers, network segmentation)

---

## References

- [GDPR Article 33 -- Notification of a personal data breach to the supervisory authority](https://gdpr-info.eu/art-33-gdpr/)
- [GDPR Article 34 -- Communication of a personal data breach to the data subject](https://gdpr-info.eu/art-34-gdpr/)
- [GDPR Article 4(12) -- Definition of personal data breach](https://gdpr-info.eu/art-4-gdpr/)
- [ICO: Personal data breaches guidance](https://ico.org.uk/for-organisations/report-a-breach/)
- [ICO: Self-assessment tool for breach reporting](https://ico.org.uk/for-organisations/report-a-breach/personal-data-breach-assessment/)
- [EDPB Guidelines 01/2021 on examples regarding personal data breach notification](https://www.edpb.europa.eu/our-work-tools/documents/public-consultations/2021/guidelines-012021-examples-regarding-personal_en)
