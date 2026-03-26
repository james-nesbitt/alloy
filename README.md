# ALLOY: MODULAR TEAM COORDINATION

**1. KERNEL (THE SUBSTRATE)**
Go backend managing Plugins, Messages, Security, and Events. Provides stable infra for coordination.

**2. PROJECT (THE EFFORT)**
Defined in the kernel (or `alloy-project.json`). Specifies:
- Required Plugins & Capabilities
- User Roles (Editor, Planner, Reviewer)

**3. FRONTEND (THE GATEWAY)**
Interaction layer (TUI, GUI, Web). Handles IPC and Hardware.

**4. WORKSPACE (THE SYNTHESIS)**
The frontend's merge of **Project Tools** (shared) and **User Tools** (private) into one interface.
