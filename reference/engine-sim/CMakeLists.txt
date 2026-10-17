cmake_minimum_required(VERSION 3.16)

# Headless build of the engine-sim core library and script interpreter.
# Upstream CMakeLists.txt pulls in delta-studio, Discord and googletest
# (FetchContent), so this file lists the needed upstream sources directly.

project(engine-sim-render CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

if (NOT CMAKE_BUILD_TYPE)
    set(CMAKE_BUILD_TYPE Release)
endif ()

set(ENGINE_SIM_SOURCE_DIR "" CACHE PATH "Patched engine-sim source tree with submodules")
if (NOT EXISTS "${ENGINE_SIM_SOURCE_DIR}/src/synthesizer.cpp")
    message(FATAL_ERROR "Set ENGINE_SIM_SOURCE_DIR to the engine-sim source tree")
endif ()

set(ES "${ENGINE_SIM_SOURCE_DIR}")
set(SUB "${ES}/dependencies/submodules")

# MSVC keyword used throughout upstream headers.
add_compile_definitions(__forceinline=inline ATG_ENGINE_SIM_PIRANHA_ENABLED)

# FindBoost module, removed in CMake 3.30 but still used here.
if (POLICY CMP0167)
    cmake_policy(SET CMP0167 OLD)
endif ()

# simple-2d-constraint-solver
file(GLOB SCS_SOURCES "${SUB}/simple-2d-constraint-solver/src/*.cpp")
add_library(simple-2d-constraint-solver STATIC ${SCS_SOURCES})

# csv-io
add_library(csv-io STATIC
    "${SUB}/csv-io/src/information.cpp"
    "${SUB}/csv-io/src/csv_data.cpp")
target_compile_definitions(csv-io PRIVATE PROJECT_NAMESPACE=atg_csv)

# piranha
find_package(FLEX REQUIRED)
find_package(BISON REQUIRED)
find_package(Boost REQUIRED COMPONENTS filesystem)

flex_target(lexer
    "${SUB}/piranha/flex-bison/scanner.l"
    "${CMAKE_CURRENT_BINARY_DIR}/scanner.auto.cpp"
    DEFINES_FILE "${CMAKE_CURRENT_BINARY_DIR}/scanner.auto.h")
bison_target(parser
    "${SUB}/piranha/flex-bison/specification.y"
    "${CMAKE_CURRENT_BINARY_DIR}/parser.auto.cpp"
    DEFINES_FILE "${CMAKE_CURRENT_BINARY_DIR}/parser.auto.h")
add_flex_bison_dependency(lexer parser)

file(GLOB PIRANHA_SOURCES "${SUB}/piranha/src/*.cpp")
add_library(piranha STATIC
    ${PIRANHA_SOURCES}
    ${BISON_parser_OUTPUTS}
    ${FLEX_lexer_OUTPUTS})
target_include_directories(piranha PUBLIC
    "${CMAKE_CURRENT_BINARY_DIR}"
    "${SUB}/piranha/dependencies/libraries/flex/include"
    "${SUB}/piranha/include")
target_link_libraries(piranha PUBLIC Boost::filesystem)

# engine-sim core: the source list of the upstream engine-sim target.
set(ENGINE_SIM_CORE
    audio_buffer camshaft crankshaft combustion_chamber connecting_rod
    convolution_filter cylinder_bank cylinder_head delay_filter
    derivative_filter direct_throttle_linkage dynamometer engine
    exhaust_system feedback_comb_filter filter fuel function gas_system
    gaussian_filter governor ignition_module impulse_response intake
    jitter_filter leveling_filter low_pass_filter part piston
    piston_engine_simulator simulator standard_valvetrain starter_motor
    synthesizer throttle transmission utilities valvetrain vehicle
    vehicle_drag_constraint vtec_valvetrain)
list(TRANSFORM ENGINE_SIM_CORE PREPEND "${ES}/src/")
list(TRANSFORM ENGINE_SIM_CORE APPEND ".cpp")
add_library(engine-sim STATIC ${ENGINE_SIM_CORE})
target_include_directories(engine-sim PUBLIC "${SUB}")
target_link_libraries(engine-sim PUBLIC simple-2d-constraint-solver csv-io)

# engine-sim script interpreter
file(GLOB INTERPRETER_SOURCES "${ES}/scripting/src/*.cpp")
add_library(engine-sim-script-interpreter STATIC ${INTERPRETER_SOURCES})
target_include_directories(engine-sim-script-interpreter PUBLIC "${SUB}")
target_link_libraries(engine-sim-script-interpreter PUBLIC engine-sim piranha)

# Headless renderer
find_package(Threads REQUIRED)
add_executable(engine-sim-render engine_sim_render.cpp)
target_include_directories(engine-sim-render PRIVATE "${ES}")
target_link_libraries(engine-sim-render PRIVATE
    engine-sim-script-interpreter engine-sim Threads::Threads)

target_compile_definitions(engine-sim-render PRIVATE
    ENGINE_SIM_DATA_DIR="${CMAKE_INSTALL_PREFIX}/share/engine-sim")

install(TARGETS engine-sim-render RUNTIME DESTINATION bin)
install(DIRECTORY "${ES}/es" DESTINATION share/engine-sim)
install(DIRECTORY "${ES}/assets/engines" DESTINATION share/engine-sim)
