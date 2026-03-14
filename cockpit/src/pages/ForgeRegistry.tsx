/**
 * ForgeRegistry — Enhanced agent registry page with pack grouping & search
 *
 * Organizes agents into pack sections (Core Pack, Custom Agents, Enterprise Pack)
 * with search, sort, and stat cards.
 */

import React, { useState, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAgents, type AgentDefinition } from '../hooks/useAgents';
import AgentCard from '../components/AgentCard';

// ============================================================================
// Types
// ============================================================================

type SortOption = 'name' | 'recently-updated' | 'category';

export interface ForgeRegistryProps {
    onDeploy?: (agent: AgentDefinition) => void;
    onConfigure?: (agent: AgentDefinition) => void;
    onDetails?: (agent: AgentDefinition) => void;
    onClone?: (agent: AgentDefinition) => void;
}

interface PackSection {
    id: string;
    title: string;
    agents: AgentDefinition[];
    showEmpty?: boolean;
}

// ============================================================================
// Styles (inline for self-containment)
// ============================================================================

const styles: Record<string, React.CSSProperties> = {
    container: {
        padding: '24px',
        maxWidth: '1400px',
        margin: '0 auto',
    },
    header: {
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'flex-start',
        marginBottom: '24px',
        flexWrap: 'wrap',
        gap: '16px',
    },
    headerLeft: {
        display: 'flex',
        alignItems: 'center',
        gap: '12px',
    },
    title: {
        fontSize: '28px',
        fontWeight: 600,
        margin: 0,
        color: 'var(--text-primary)',
    },
    headerActions: {
        display: 'flex',
        gap: '12px',
    },
    searchContainer: {
        marginBottom: '24px',
        maxWidth: '400px',
    },
    searchInput: {
        width: '100%',
        padding: '10px 16px',
        paddingLeft: '40px',
        borderRadius: '8px',
        border: '1px solid var(--border)',
        background: 'var(--bg-elevated)',
        color: 'var(--text-primary)',
        fontSize: '14px',
        outline: 'none',
        transition: 'border-color 0.2s, box-shadow 0.2s',
    },
    searchWrapper: {
        position: 'relative' as const,
    },
    searchIcon: {
        position: 'absolute' as const,
        left: '12px',
        top: '50%',
        transform: 'translateY(-50%)',
        color: 'var(--text-muted)',
    },
    statsRow: {
        display: 'grid',
        gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))',
        gap: '16px',
        marginBottom: '24px',
    },
    statCard: {
        background: 'var(--glass-bg)',
        backdropFilter: 'blur(12px)',
        borderRadius: '12px',
        border: '1px solid var(--glass-border)',
        padding: '20px',
        display: 'flex',
        flexDirection: 'column',
        gap: '8px',
    },
    statLabel: {
        fontSize: '13px',
        color: 'var(--text-muted)',
        fontWeight: 500,
        textTransform: 'uppercase' as const,
        letterSpacing: '0.5px',
    },
    statValue: {
        fontSize: '32px',
        fontWeight: 700,
        color: 'var(--text-primary)',
    },
    sortRow: {
        display: 'flex',
        justifyContent: 'flex-end',
        marginBottom: '24px',
    },
    sortSelect: {
        padding: '8px 32px 8px 12px',
        borderRadius: '6px',
        border: '1px solid var(--border)',
        background: 'var(--bg-elevated)',
        color: 'var(--text-primary)',
        fontSize: '14px',
        cursor: 'pointer',
        outline: 'none',
        appearance: 'none',
        backgroundImage: `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%236b7280' stroke-width='2'%3E%3Cpath d='M6 9l6 6 6-6'/%3E%3C/svg%3E")`,
        backgroundRepeat: 'no-repeat',
        backgroundPosition: 'right 10px center',
    },
    packSection: {
        marginBottom: '32px',
    },
    packHeader: {
        display: 'flex',
        alignItems: 'center',
        gap: '12px',
        marginBottom: '16px',
        paddingBottom: '12px',
        borderBottom: '1px solid var(--border)',
    },
    packTitle: {
        fontSize: '18px',
        fontWeight: 600,
        color: 'var(--text-primary)',
        margin: 0,
    },
    packBadge: {
        background: 'var(--accent)',
        color: 'white',
        fontSize: '12px',
        fontWeight: 600,
        padding: '2px 8px',
        borderRadius: '12px',
    },
    agentsGrid: {
        display: 'grid',
        gridTemplateColumns: 'repeat(auto-fill, minmax(320px, 1fr))',
        gap: '16px',
    },
    emptyState: {
        textAlign: 'center',
        padding: '40px 20px',
        background: 'var(--bg-elevated)',
        borderRadius: '12px',
        border: '1px dashed var(--border)',
    },
    emptyTitle: {
        fontSize: '16px',
        fontWeight: 500,
        color: 'var(--text-primary)',
        marginBottom: '8px',
    },
    emptyDesc: {
        fontSize: '14px',
        color: 'var(--text-muted)',
        marginBottom: '16px',
    },
    iconBtn: {
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
    },
};

// ============================================================================
// Sub-Components
// ============================================================================

/** Search icon SVG */
const SearchIcon: React.FC<{ size?: number }> = ({ size = 18 }) => (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <circle cx="11" cy="11" r="8" />
        <line x1="21" y1="21" x2="16.65" y2="16.65" />
    </svg>
);

/** Plus icon SVG */
const PlusIcon: React.FC<{ size?: number }> = ({ size = 16 }) => (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <line x1="12" y1="5" x2="12" y2="19" />
        <line x1="5" y1="12" x2="19" y2="12" />
    </svg>
);

/** Upload icon SVG */
const UploadIcon: React.FC<{ size?: number }> = ({ size = 16 }) => (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
        <polyline points="17 8 12 3 7 8" />
        <line x1="12" y1="3" x2="12" y2="15" />
    </svg>
);

/** Robot icon SVG for header */
const RobotIcon: React.FC<{ size?: number }> = ({ size = 28 }) => (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <rect x="3" y="11" width="18" height="10" rx="2" />
        <circle cx="12" cy="5" r="2" />
        <path d="M12 7v4" />
        <line x1="8" y1="16" x2="8" y2="16" />
        <line x1="16" y1="16" x2="16" y2="16" />
    </svg>
);

/** Chevron icon for expand/collapse */
const ChevronIcon: React.FC<{ expanded: boolean; size?: number }> = ({ expanded, size = 20 }) => (
    <svg
        width={size}
        height={size}
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
        style={{
            transform: expanded ? 'rotate(180deg)' : 'rotate(0deg)',
            transition: 'transform 0.2s',
        }}
    >
        <polyline points="6 9 12 15 18 9" />
    </svg>
);

// ============================================================================
// ForgeRegistry Component
// ============================================================================

const ForgeRegistry: React.FC<ForgeRegistryProps> = ({ onDeploy, onConfigure, onDetails, onClone }) => {
    const navigate = useNavigate();

    // State
    const [searchQuery, setSearchQuery] = useState('');
    const [sortOption, setSortOption] = useState<SortOption>('name');
    const [expandedSections, setExpandedSections] = useState<Record<string, boolean>>({
        'pack-core': true,
        'pack-custom': true,
        'pack-enterprise': true,
    });

    // Data
    const { agents, isLoading, error, refetch } = useAgents();

    // Filter and sort agents
    const filteredAndSortedAgents = useMemo(() => {
        let filtered = agents;

        // Apply search filter
        if (searchQuery.trim()) {
            const query = searchQuery.toLowerCase();
            filtered = agents.filter(
                (a) =>
                    (a.display_name || a.name).toLowerCase().includes(query) ||
                    (a.description || '').toLowerCase().includes(query) ||
                    a.name.toLowerCase().includes(query)
            );
        }

        // Apply sorting
        return [...filtered].sort((a, b) => {
            switch (sortOption) {
                case 'name':
                    return (a.display_name || a.name).localeCompare(b.display_name || b.name);
                case 'recently-updated':
                    return new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime();
                case 'category':
                    return (a.category || '').localeCompare(b.category || '');
                default:
                    return 0;
            }
        });
    }, [agents, searchQuery, sortOption]);

    // Group agents into packs
    const packSections = useMemo((): PackSection[] => {
        const core = filteredAndSortedAgents.filter((a) => a.is_builtin === true);
        const custom = filteredAndSortedAgents.filter((a) => a.is_builtin === false && a.is_enterprise !== true);
        const enterprise = filteredAndSortedAgents.filter((a) => a.is_enterprise === true);

        const sections: PackSection[] = [
            { id: 'pack-core', title: 'Core Pack', agents: core },
            { id: 'pack-custom', title: 'Custom Agents', agents: custom },
        ];

        // Only add enterprise section if there are enterprise agents
        if (enterprise.length > 0) {
            sections.push({ id: 'pack-enterprise', title: 'Enterprise Pack (Coming Soon)', agents: enterprise });
        }

        return sections;
    }, [filteredAndSortedAgents]);

    // Calculate stats
    const stats = useMemo(() => {
        const total = agents.length;
        const customCount = agents.filter((a) => a.is_builtin === false).length;
        const coreCount = agents.filter((a) => a.is_builtin === true).length;
        // Recent runs - for now we can show 0 as the task mentions it can be 0 initially
        // In the future this could be populated from agent runs data
        const recentRuns = 0;

        return { total, customCount, coreCount, recentRuns };
    }, [agents]);

    // Toggle section expand/collapse
    const toggleSection = (sectionId: string) => {
        setExpandedSections((prev) => ({
            ...prev,
            [sectionId]: !prev[sectionId],
        }));
    };

    // Handle import button click (placeholder)
    const handleImport = () => {
        // TODO: Implement file picker for importing agents
        console.log('Import agent clicked');
    };

    // Handlers for AgentCard actions — delegate to parent via props
    const handleDeploy = (agent: AgentDefinition) => {
        onDeploy?.(agent);
    };

    const handleConfigure = (agent: AgentDefinition) => {
        onConfigure?.(agent);
    };

    const handleDetails = (agent: AgentDefinition) => {
        onDetails?.(agent);
    };

    // Render loading state
    if (isLoading) {
        return (
            <div style={styles.container}>
                <div style={styles.header}>
                    <div style={styles.headerLeft}>
                        <span style={styles.iconBtn}><RobotIcon /></span>
                        <h1 style={styles.title}>Agent Forge</h1>
                    </div>
                </div>
                <div style={styles.statsRow}>
                    {[1, 2, 3, 4].map((i) => (
                        <div key={i} style={styles.statCard}>
                            <div style={{ ...styles.statLabel, opacity: 0.5 }}>Loading...</div>
                            <div style={{ ...styles.statValue, opacity: 0.3 }}>--</div>
                        </div>
                    ))}
                </div>
            </div>
        );
    }

    // Render error state
    if (error) {
        return (
            <div style={styles.container}>
                <div style={styles.header}>
                    <div style={styles.headerLeft}>
                        <span style={styles.iconBtn}><RobotIcon /></span>
                        <h1 style={styles.title}>Agent Forge</h1>
                    </div>
                </div>
                <div style={styles.emptyState}>
                    <div style={styles.emptyTitle}>Error Loading Agents</div>
                    <div style={styles.emptyDesc}>{error.message}</div>
                    <button className="btn btn--primary btn--sm" onClick={() => refetch()}>
                        Retry
                    </button>
                </div>
            </div>
        );
    }

    return (
        <div style={styles.container}>
            {/* Page Header */}
            <div id="forge-registry-header" style={styles.header}>
                <div style={styles.headerLeft}>
                    <span style={styles.iconBtn}>
                        <RobotIcon />
                    </span>
                    <h1 style={styles.title}>Agent Forge</h1>
                </div>
                <div style={styles.headerActions}>
                    <button
                        className="btn btn--primary btn--sm"
                        onClick={() => navigate('/forge/builder')}
                    >
                        <PlusIcon size={14} />
                        Create Agent
                    </button>
                    <button
                        className="btn btn--outline btn--sm"
                        onClick={handleImport}
                    >
                        <UploadIcon size={14} />
                        Import Agent
                    </button>
                </div>
            </div>

            {/* Search Bar */}
            <div id="forge-registry-search" style={styles.searchContainer}>
                <div style={styles.searchWrapper}>
                    <span style={styles.searchIcon}>
                        <SearchIcon />
                    </span>
                    <input
                        type="text"
                        placeholder="Search agents..."
                        value={searchQuery}
                        onChange={(e) => setSearchQuery(e.target.value)}
                        style={styles.searchInput}
                    />
                </div>
            </div>

            {/* Stats Row */}
            <div style={styles.statsRow}>
                <div id="stat-total-agents" style={styles.statCard}>
                    <span style={styles.statLabel}>Total Agents</span>
                    <span style={styles.statValue}>{stats.total}</span>
                </div>
                <div id="stat-custom-agents" style={styles.statCard}>
                    <span style={styles.statLabel}>Custom Agents</span>
                    <span style={styles.statValue}>{stats.customCount}</span>
                </div>
                <div id="stat-core-agents" style={styles.statCard}>
                    <span style={styles.statLabel}>Core Agents</span>
                    <span style={styles.statValue}>{stats.coreCount}</span>
                </div>
                <div id="stat-recent-runs" style={styles.statCard}>
                    <span style={styles.statLabel}>Recent Runs</span>
                    <span style={styles.statValue}>{stats.recentRuns}</span>
                </div>
            </div>

            {/* Sort Dropdown */}
            <div style={styles.sortRow}>
                <select
                    id="forge-registry-sort"
                    value={sortOption}
                    onChange={(e) => setSortOption(e.target.value as SortOption)}
                    style={styles.sortSelect}
                >
                    <option value="name">Name (A-Z)</option>
                    <option value="recently-updated">Recently Updated</option>
                    <option value="category">Category</option>
                </select>
            </div>

            {/* Pack Sections */}
            {packSections.map((section) => (
                <div key={section.id} id={section.id} style={styles.packSection}>
                    {/* Section Header */}
                    <div
                        style={styles.packHeader}
                        onClick={() => toggleSection(section.id)}
                        className="pack-section-header"
                    >
                        <ChevronIcon expanded={expandedSections[section.id] || false} />
                        <h2 style={styles.packTitle}>{section.title}</h2>
                        <span style={styles.packBadge}>{section.agents.length}</span>
                    </div>

                    {/* Section Content */}
                    {expandedSections[section.id] && (
                        section.agents.length > 0 ? (
                            <div style={styles.agentsGrid}>
                                {section.agents.map((agent) => (
                                    <AgentCard
                                        key={agent.id}
                                        agent={agent}
                                        onDeploy={handleDeploy}
                                        onConfigure={handleConfigure}
                                        onDetails={handleDetails}
                                        onClone={onClone}
                                    />
                                ))}
                            </div>
                        ) : (
                            <div style={styles.emptyState}>
                                <div style={styles.emptyTitle}>No agents in this pack</div>
                                <div style={styles.emptyDesc}>
                                    {section.id === 'pack-core'
                                        ? 'Core agents are built-in and cannot be added manually.'
                                        : 'Create your first agent to get started.'}
                                </div>
                                {section.id !== 'pack-core' && (
                                    <button
                                        className="btn btn--primary btn--sm"
                                        onClick={() => navigate('/forge/builder')}
                                    >
                                        <PlusIcon size={14} />
                                        Create Agent
                                    </button>
                                )}
                            </div>
                        )
                    )}
                </div>
            ))}
        </div>
    );
};

export default ForgeRegistry;
