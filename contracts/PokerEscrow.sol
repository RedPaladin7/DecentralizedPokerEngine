// SPDX-License-Identifier: MIT 
pragma solidity ^0.8.20;

contract PokerEscrow {
    uint256 public constant SETTLEMENT_DEADLINE = 1000;
    uint256 public constant CHALLENGE_WINDOW = 50;
    uint256 public constant SIG_THRESHOLD_NUM = 2;
    uint256 public constant SIG_THRESHOLD_DEN = 3;

    uint256 public constant SLASH_BURN_BPS = 2000;

    enum TableState {
        Open, 
        Playing,
        Settled,
        Disputed,
        Abandoned
    }

    struct Player {
        address payable addr;
        string peerID;
        uint256 buyIn;
        bool withdrawn;
        bool slashed;
    }

    struct PotResult {
        uint256 amount;
        address[] winners;
    }

    address public immutable owner;
    string public tableID;
    uint8 public maxSeats;
    uint256 public totalEscrow;

    TableState public state;
    uint256 public gameStartBlock;
    uint256 public settlementBlock;
    bytes32 public stateRoot;

    Player[] public players;
    mapping(address => uint8) public seatOf;
    mapping(address => bytes) public slashEvidence;

    event PlayerJoined(address indexed player, string peerID, uint256 amount, uint8 seat);
    event GameStarted(uint256 blockNumber);
    event OutcomeReported(bytes32 stateRoot, uint256 blockNumber);
    event PayoutSent(address indexed player, uint256 amount);
    event DisputeFiled(address indexed filer, address indexed accused, string reason);
    event SlashExecuted(address indexed player, uint256 slashedAmount, uint256 burnedAmount);
    event Refunded(address indexed player, uint256 amount);
    event Abandoned(uint256 blockNumber);

    modifier inState(TableState expected) {
        require(state == expected, "PokerEscrow: wrong table state");
        _;
    }

    modifier onlySeated() {
        require(seatOf[msg.sender] != 0, "PokerEscrow: caller not seated");
        _; 
    }

    constructor(string memory _tableID, uint8 _maxSeats) {
        require(_maxSeats >= 2 && _maxSeats <= 9, "PokerEscrow: invalid max seats");
        owner = msg.sender;
        tableID = _tableID;
        maxSeats = _maxSeats;
        state = TableState.Open;
    }

    function joinTable(string calldata peerID) external payable inState(TableState.Open) {
        require(players.length < maxSeats, "PokerEscrow: table full");
        require(msg.value > 0, "PokerEscrow: must deposit");
        require(seatOf[msg.sender] == 0, "PokerEscrow: already seated");
        require(bytes(peerID).length > 0, "PokerEscrow: invalid peerID");

        uint8 seat = uint8(players.length);
        players.push(Player({
            addr: payable(msg.sender),
            peerID: peerID,
            buyIn: msg.value,
            withdrawn: false,
            slashed: false
        }));
        seatOf[msg.sender] = seat + 1;
        totalEscrow += msg.value;
        emit PlayerJoined(msg.sender, peerID, msg.value, seat);

        if (players.length == maxSeats) {
            gameStartBlock = block.number;
            state = TableState.Playing;
            emit GameStarted(gameStartBlock);
        }
    }

    function reportOutcome(
        int256[] calldata payoutDeltas,
        bytes32 _stateRoot,
        bytes[] calldata signatures,
        uint256 handNum
    ) external inState(TableState.Playing) onlySeated {
        require(payoutDeltas.length == players.length, "PokerEscrow: invalid payout deltas");
        require(_verifyChipConservation(payoutDeltas), "PokerEscrow: invalid chip conservation");
        require(signatures.length >= _requiredSigs(), "PokerEscrow: not enough signatures");

        bytes32 digest = _outcomeDigest(handNum, payoutDeltas, _stateRoot);
        _verifySignatures(digest, signatures);

        stateRoot = _stateRoot;
        state = TableState.Settled;
        settlementBlock = block.number;

        emit OutcomeReported(_stateRoot, block.number);

        _executePayouts(payoutDeltas);
    }

    function submitDispute(
        address accused,
        string calldata reason,
        bytes calldata evidence,
        bytes calldata accuserSig
    ) external inState(TableState.Settled) onlySeated {
        require(block.number <= settlementBlock + CHALLENGE_WINDOW, "PokerEscrow: dispute deadline exceeded");
        require(seatOf[accused] != 0, "PokerEscrow: accused not seated");
        require(!players[seatOf[accused]-1].slashed, "PokerEscrow: accused already slashed");
        require(evidence.length > 0, "PokerEscrow: invalid evidence");

        bytes32 claimHash = keccak256(abi.encode(accused, reason, evidence));
        require(_recoverSigner(claimHash, accuserSig)== msg.sender, "PokerEscrow: invalid accuser signature");

        state = TableState.Disputed;
        slashEvidence[accused] = evidence;

        emit DisputeFiled(msg.sender, accused, reason);

        _executeSlash(accused);
    }

    function markAbandoned() external inState(TableState.Playing) {
        require (block.number > gameStartBlock + SETTLEMENT_DEADLINE, "PokerEscrow: deadline exceeded");
        state = TableState.Abandoned;
        emit Abandoned(block.number);
    }

    function refund() external inState(TableState.Abandoned) onlySeated {
        uint8 idx = seatOf[msg.sender] - 1;
        Player storage p = players[idx];
        require(!p.withdrawn, "PokerEscrow: already withdrawn");
        p.withdrawn = true;
        uint256 amount = p.buyIn;
        totalEscrow -= amount;
        p.addr.transfer(amount);
        emit Refunded(msg.sender, amount);
    }

    function playerCount() external view returns(uint256) {
        return players.length;
    }

    function playerAt(uint8 seat) external view returns (
        address addr, string memory peerID, uint256 buyIn, bool withdrawn, bool slashed) {
        require (seat < players.length, "PokerEscrow: invalid seat");
        Player storage p = players[seat];
        return (p.addr, p.peerID, p.buyIn, p.withdrawn, p.slashed);
    }

    function requiredSignatures() external view returns (uint256) {
        return _requiredSigs();
    }

    function _requiredSigs() internal view returns (uint256) {
        uint256 n = players.length;
        return (n * SIG_THRESHOLD_NUM + SIG_THRESHOLD_DEN - 1) / SIG_THRESHOLD_DEN;
    }

    function _verifyChipConservation(int256[] calldata deltas) internal pure returns (bool) {
        int256 sum = 0;
        for (uint256 i = 0; i < deltas.length; i++) {
            sum += deltas[i];
        }
        return sum == 0;
    }

    function _outcomeDigest(uint256 handNum, int256[] calldata payoutDeltas, bytes32 _stateRoot) internal view returns (bytes32) {
        return keccak256(abi.encode(
            tableID,
            handNum,
            payoutDeltas,
            _stateRoot
        ));
    }

    function _verifySignatures(bytes32 digest, bytes[] calldata sigs) internal view {
        bytes32 ethHash = keccak256(abi.encodePacked(
            "\x19Ethereum Signed Message:\n32",
            digest
        ));
        address[] memory seen = new address[](sigs.length);
        uint256 validCount = 0;

        for (uint256 i = 0; i < sigs.length; i++) {
            address signer = _recoverSigner(ethHash, sigs[i]);
            if (seatOf[signer] != 0 && !_contains(seen, validCount, signer)) {
                seen[validCount] = signer;
                validCount++;
            }
        }
    }

    function _recoverSigner(bytes32 hash, bytes calldata sig) internal pure returns (address) {
        require(sig.length == 65, "PokerEscrow: invalid signature length");
        bytes32 r;
        bytes32 s;
        uint8 v;
        assembly {
            r := calldataload(sig.offset)
            s := calldataload(add(sig.offset, 32))
            v := byte(0, calldataload(add(sig.offset, 64)))
        }
        if (v < 27) v += 27;
        require(v == 27 || v == 28, "PokerEscrow: invalid v");
        return ecrecover(hash, v, r, s);
    }

    function _contains(address[] memory arr, uint256 len, address target) internal pure returns (bool) {
        for (uint256 i = 0; i < len; i++) {
            if (arr[i] == target) {
                return true;
            }
        }
        return false;
    }

    function _executePayouts(int256[] calldata deltas) internal {
        for (uint256 i = 0; i < players.length; i++) {
            Player storage p = players[i];
            if (p.withdrawn) continue;

            int256 delta = deltas[i];
            int256 buyIn = int256(p.buyIn);
            int256 payout = buyIn + delta;

            if (payout <= 0) {
                p.withdrawn = true;
                continue;
            }

            uint256 amount = uint256(payout);
            p.withdrawn = true;
            totalEscrow -= amount;
            p.addr.transfer(amount);
            emit PayoutSent(p.addr, amount);
        }
    }

    function _executeSlash(address accused) internal {
        uint8 idx = seatOf[accused] - 1;
        Player storage p = players[idx];
        p.slashed = true;

        uint256 slashable = p.buyIn;

        uint256 burnAmount = (slashable * SLASH_BURN_BPS) / 10000;
        uint256 redistributed = slashable - burnAmount;

        uint256 eligible = 0;
        for (uint256 i = 0; i < players.length; i++) {
            if (!players[i].slashed && !players[i].withdrawn && players[i].addr != accused) {
                eligible++;
            }
        }

        if (eligible > 0) {
            uint256 share = redistributed / eligible;
            for (uint256 i = 0; i < players.length; i++) {
                Player storage r = players[i];
                if(!r.slashed && !r.withdrawn && r.addr != accused) {
                    r.addr.transfer(share);
                    emit PayoutSent(r.addr, share);
                }
            }
        }

        emit SlashExecuted(accused, redistributed, burnAmount);
        state = TableState.Settled;
    }

    receive() external payable {
        revert("PokerEscrow: use joinTable()");
    }
}